# Plan: Secret Booking Link (Free Studio-Artist Bookings)

## Goal

A settings toggle that, when enabled, exposes a **secret link** allowing studio artists to book:

- **Free** — no card, no Stripe, no charge, ever.
- **Last-minute** — the minimum-lead-time rule ("bookings must be made at least N hours in advance") does not apply.
- **Configurable 24/7 access** — a second setting toggle ("Allow bookings outside regular hours") unlocks all 24 hours of the day (`00:00–24:00`). Regular booking hours are bypassed; closures and existing bookings still apply.

---

## Architectural Principles (Avoiding Tech Debt)

1. **No duplicate endpoints:** Extend existing public endpoints (`/api/availability`, `/api/availability/days`, `/api/bookings`) with secret-token awareness rather than creating parallel `/api/link/*` routes. Add only one small verification endpoint: `GET /api/secret-link/verify?token=...`.
2. **No duplicate page components:** Reuse `BookingPage.tsx` directly under `/book/:token` via react-router. Conditionally hide Stripe Elements, pricing disclosures, and gate questions based on token state.
3. **No duplicate SQL insert queries:** Update `CreateBooking` in `bookings.sql` and `queries.go` to accept nullable Stripe fields and a `via_secret_link` boolean.
4. **Token transport over header/query only:** `X-Secret-Token` header, or `?secret_token=` query param on GETs, so tokens stay out of URL *path* logs.
   - In `internal/api/server.go`, add `X-Secret-Token` to `cors.Options.AllowedHeaders` so dev/separate-origin frontend callers are not blocked.
5. **Full 24/7 availability handling:** When after-hours is enabled for the secret link, slots span `00:00` to `24:00`. `TimeSlotPicker` handles contiguous multi-hour runs naturally.
6. **Rate limits (decided: no bypass):** Secret-link bookings pass through the same `limitBookingCreate` IP budget as everyone else. The limit is 20 creates/hour/IP — far above artist demand — and keeping a single code path avoids carving security-relevant exceptions. The token is not a license to write unbounded rows if it leaks.
7. **Public settings stay ignorant:** `GET /api/settings/public` must **not** expose any `secret_link` key — the link's existence is private. Pinned as an explicit test.
8. **Idempotency note:** The free flow has no `setup_intent_id` idempotency key. A network retry after a succeeded create will 409 ("no longer available") instead of returning the booking. Harmless by design, but the link-mode frontend maps that specific 409-on-retry to a success-ish "already booked" UX.

---

## Database Changes (next migration — `018` if `017` lands first)

> **Numbering is contingent, not fixed.** Migrations `015` (prepare attempts), `016` (charge idempotency), and `017` (create attempts) exist as *uncommitted local work* right now (untracked/unstaged in this tree). Use the next contiguous number **at merge time** and coordinate with whoever lands 017. The examples below assume `018`.

```sql
ALTER TABLE settings
  ADD COLUMN secret_link_enabled         BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN secret_link_token           TEXT,          -- NULL until first enable/rotate
  ADD COLUMN secret_link_any_hours       BOOLEAN NOT NULL DEFAULT FALSE;

CREATE UNIQUE INDEX settings_secret_link_token_key
  ON settings (secret_link_token) WHERE secret_link_token IS NOT NULL;

ALTER TABLE bookings
  ADD COLUMN via_secret_link BOOLEAN NOT NULL DEFAULT FALSE;
```

`*_secret_link.down.sql` drops the index and removes the four added columns.

**Also update** `internal/db/models.go` (`Settings`, `Booking` structs) and `internal/db/queries.go` (`bookingColumns` / `scanBooking`) for the new fields — every read query breaks silently if `scanBooking` is forgotten.

*`tests/scripts/reset-db.sh` needs no change: `INSERT INTO settings DEFAULT VALUES` already restores the NOT NULL DEFAULT columns.*

---

## Backend Changes

### 1. Token Verification & Helpers (`internal/api/secret_link.go` or `helpers.go`)

- **Extract Token:** Helper `secretTokenFromRequest(r *http.Request) string` checks `X-Secret-Token` header, falling back to query param `secret_token`.
- **Validate Token:** Helper `validateSecretToken(ctx, settings, token) bool`:
  - Returns `true` iff `settings.SecretLinkEnabled == true` AND `settings.SecretLinkToken != nil` AND constant-time string comparison matches the token.
- **New endpoint:** `GET /api/secret-link/verify?token=...`
  - Returns `{ "valid": true, "resource_name": settings.ResourceName, "any_hours": settings.SecretLinkAnyHours }` if active.
  - Returns `200` with `{ "valid": false }` if disabled or token mismatch (SPA-friendly; no reason to 404 a *probe*, and a uniform 200 keeps the contract simple — the decision is pinned here for CONTRACT.md).

### 2. Availability Refactoring (`internal/api/public.go`)

Unify availability calculation with an options struct:

```go
type availabilityOpts struct {
    trainerHours   bool // new-user gate windows
    anyHours       bool // 24/7 mode: full day 00:00–24:00
    bypassLeadTime bool // skip earliestBookableStart (lead hours) check
}
```

- When `secretToken` is valid:
  - `bypassLeadTime = true` (slots right up to the current hour become available).
  - If `settings.SecretLinkAnyHours == true`: `anyHours = true` (slots generated across the full 24-hour day in the configured timezone).
- When `secretToken` is absent or invalid:
  - Standard behavior (`earliestBookableStart` applied, normal `bookable_start`/`bookable_end` bounds, new-user gate active).

### 3. Unified Booking Creation (`internal/api/public.go` & `internal/db/`)

Update `CreateBooking` query signature to handle both standard and secret-link bookings:

```sql
-- name: CreateBooking :one
INSERT INTO bookings (
    name, email, metadata, start_time, end_time,
    stripe_setup_intent_id, stripe_payment_method_id,
    is_new_user, via_secret_link
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;
```

In `handleCreateBooking` (`internal/api/public.go`):
- Read `secretToken` from request (`X-Secret-Token` or JSON body `secret_token`).
- If token is valid:
  - `viaSecretLink = true`.
  - Stripe parameters (`setup_intent_id`, payment method lookup) are **skipped**.
  - New-user gate and new-customer restriction checks are **skipped**.
  - Validation bypasses minimum lead time. If `SecretLinkAnyHours` is enabled, validation also skips `bookingWithinBookableHours`.
  - Durations (min/max), closure overlaps, past start, and slot availability (`bookings_no_overlap` constraint) remain strictly enforced.
- If no token:
  - Existing flow runs unchanged (requires valid `setup_intent_id`, confirms payment method, enforces lead time, gate, etc.).

### 4. Booking Lifecycle & Schedulers

Fix existing card-assumption guards so free bookings don't get stranded:

1. **`CompleteExpiredBookings` (`internal/db/queries.go`):**
   ```sql
   UPDATE bookings SET status = 'completed', completed_at = NOW()
   WHERE status = 'confirmed'
     AND end_time < NOW()
     AND (stripe_payment_method_id IS NOT NULL OR via_secret_link = TRUE)
   RETURNING ...
   ```
2. **`handleAdminUpdateBooking` (`internal/api/admin.go`):**
   - When marking `status = completed`, only enforce `StripePaymentMethodID != nil` if `!booking.ViaSecretLink`.
3. **`handleAdminChargeBooking` (`internal/api/admin.go:~440-460`): must early-out with 400, never dereference:**
   - `s.stripe.ChargePaymentMethod(*booking.StripePaymentMethodID, ...)` panics on free bookings (NULL payment method). Before any dereference:
     ```go
     if booking.ViaSecretLink || booking.StripePaymentMethodID == nil {
         s.writeError(w, r, http.StatusBadRequest, "free studio booking cannot be charged", nil)
         return
     }
     ```
4. **Admin UI (`AdminBookings.tsx`): free bookings render distinctly and are not chargeable from the UI.**
   - "Free · Studio booking" badge (distinct styling, not just the `New user` badge reused) so staff can tell at a glance which rows are artist blocks vs external paid bookings.
   - The Charge action is **hidden/disabled** on free bookings (the API 400 above is the backstop, not the primary UX).
5. **Auto-charge safety:**
   - Existing scheduler queries already require `stripe_payment_method_id IS NOT NULL` and `stripe_payment_intent_id IS NULL`, so free bookings are automatically never claimed for charge.
6. **Insights audit & compatibility:**
   - Verified `GetBookingInsights` queries in `internal/db/queries.go`:
     - `TotalRevenueCents`: Uses `COALESCE(SUM(amount_cents), 0)` filtered on `status = 'charged' AND amount_cents IS NOT NULL`. Free bookings with `NULL` amount will safely contribute `$0.00` to revenue without causing SQL NULL scanning errors.
     - `CustomerInsight` rows: `revenue_cents` is wrapped in `COALESCE(SUM(...) FILTER (...), 0)` which is safe.
     - `BookingsByStatus`: Simply groups by status enum.
     - `ReferralSources`: Free bookings omit referral questions, which are already filtered out cleanly (`metadata->>'ReferralSource' IS NOT NULL AND ... != ''`).
   - Frontend `AdminInsights.tsx`: Correctly formats `$0.00` for revenue and handles any status group. Will not crash or divide by zero.

### 5. Admin Settings & Rotation (`internal/api/admin.go`)

- **Settings mapping:** Update `settingsToMap` and `handleAdminUpdateSettings` to support:
  ```json
  "secret_link": {
    "enabled": true,
    "allow_outside_hours": true,
    "token": "...",
    "url": "https://example.com/book/..."
  }
  ```
- **Rotate Endpoint:** `POST /api/admin/secret-link/rotate`
  - Generates 32 random bytes (hex-encoded string, 64 chars).
  - Saves to `settings.secret_link_token`.
  - Returns `{ "token": "...", "url": "..." }`.
  - Old link immediately returns 404.

### 6. Email Template Updates (`internal/email/`)

- Add `FreeBooking bool` to `ConfirmationData` and `StaffNewBookingData`.
- In `confirmation.html`: Conditionally render "Studio booking - no payment required" instead of the "Payment will be collected after your session" / card-on-file wording.
- In `staff_new_booking.html`: Add a `[Free Studio Booking]` tag next to booker details.

---

## Frontend Changes

### 1. SPA Route Configuration (`web/src/App.tsx` & `internal/api/spa.go`)

- In `internal/api/spa.go`: Add `strings.HasPrefix(p, "/book/")` to `isSPARoute()` so deep links don't 404.
- In `web/src/App.tsx`: Route `/book/:secretToken` directly to `<BookingPage />`.

### 2. Single Dynamic `BookingPage.tsx`

`BookingPage.tsx` extracts `const { secretToken } = useParams()`:
- **On mount if `secretToken` exists:**
  - Calls `/api/secret-link/verify?token=...`.
  - If invalid: renders an error screen ("This booking link is invalid or has expired").
  - If valid: flags page mode as `isSecretLink = true`.
- **UI adjustments when `isSecretLink = true`:**
  - Header displays: "Book {resource_name} (Studio Artist)" and "No payment required".
  - Hides hourly rate and payment disclosure.
  - Hides new-user gate question and referral source questions.
  - Hides Stripe Elements / Card input entirely.
  - "Bookings must be made at least N hours in advance" banner is omitted.
  - Submit button text: "Confirm Studio Booking" (submits directly without `prepareBooking` / Stripe confirmation).
- **API calls when `isSecretLink = true`:**
  - Passes `secret_token` query param to `getAvailability` and `getAvailableDates`.
  - Passes `secret_token` in `createBooking` payload (or `X-Secret-Token` header via API client).
- **Skip Stripe entirely in link mode:** do not call `loadStripe()` / render `<Elements>` — avoids the dead script load and keeps the payment code path provably absent. (Structure: `BookingPage` renders `<BookingForm>` with a `stripeReady` flag; the current Stripe wrapper moves inside the non-link branch only.)

### 3. Admin Settings UI (`web/src/pages/AdminSettings.tsx`)

In the **Booking rules** tab:
- Add a new "Secret Studio Link" card.
- Checkbox: "Enable secret booking link".
- Checkbox: "Allow bookings outside regular hours (24/7 access)" (visible when enabled).
- Link display box with one-click "Copy link" button.
- "Regenerate link" button with confirmation alert warning that previous links will be revoked.

### 4. Admin Bookings List (`web/src/pages/AdminBookings.tsx`)

- Add a "Free · Studio booking" badge (distinct styling, not the `New user` badge reused) when `booking.via_secret_link` is true, so staff can tell artist blocks from external paid bookings at a glance.
- **Money-field render branch (`BookingPaymentInfo`, ~line 259):** free bookings have `amount_cents: null` and no payment method, and the current fallback renders `—` — indistinguishable from a broken row. Render `via_secret_link` bookings as **"Free"** (never format NULL, never `$0.00`), gated on the badge condition:
  ```
  if (booking.via_secret_link) return <span className="...">Free</span>
  ```
- **Charge action:** already gated in `BookingActions` (`booking.stripe_payment_method_id &&`) — free bookings naturally hide the button. Add a Playwright assertion that no enabled Charge action appears on a free row (the API 400 is the backstop).

---

## Testing Plan

### 1. In-Process Go Tests (`internal/api/secret_link_test.go`)
- **Verification & Auth:**
  - Valid token returns 200 `{ "valid": true, ... }` with resource details.
  - Disabled setting returns `{ "valid": false }`.
  - Wrong token returns `{ "valid": false }`.
  - Admin rotation revokes old token immediately.
- **Privacy pin (security property):**
  - `GET /api/settings/public` response contains **no** `secret_link` key while the link is enabled.
- **Availability:**
  - With token: slots starting `< min_lead_hours` are marked `available: true`.
  - Without token: slots starting `< min_lead_hours` are marked `available: false`.
  - With `secret_link_any_hours = true`: slots outside bookable hours (e.g. 02:00, 22:00) appear as available.
  - With `secret_link_any_hours = false`: off-hour slots do not appear.
  - Closures and existing booking conflicts are still properly blocked.
  - **DST pin:** a link-mode availability test pinned to a spring-forward date (following the Hurl convention of anchoring DST-switch dates, e.g. `2030-03-10`), asserting slot count/timezone offset correctness.
- **Creation:**
  - Successful free booking created without setup intent or card.
  - `via_secret_link = true` in DB, stripe fields are NULL.
  - Lead time violations are accepted.
  - Multi-hour duration limits and closures are still enforced.
  - Retry-after-success returns 409 (overlap), not a duplicate row.
- **Lifecycle & charge safety:**
  - Free booking auto-completes via `CompleteExpiredBookings` without a payment method.
  - Admin can manually mark a free booking as `completed`.
  - Free booking is never auto-charged.
  - **`POST /api/admin/bookings/{id}/charge` on a free booking returns 400 "free studio booking cannot be charged" (never 500/panic).**

### 2. Functional Hurl Suites (`tests/hurl/suites/`)
The repository now runs black-box HTTP suites against the compiled binary and real Postgres/Mailpit via `make test-func-api`. We must extend:
- **`05_admin_settings_users_closures.hurl`:**
  - Enable `secret_link` and verify admin GET reflects it.
  - Call `POST /api/admin/secret-link/rotate` and assert captured token format.
  - Verify `GET /api/secret-link/verify?token={{token}}` returns 200 `valid: true`.
  - Verify invalid token returns 200 `valid: false` (uniform contract — see plan §Backend 1).
  - Toggle `allow_outside_hours` and verify round-trip.
- **`06_spa_routing.hurl`:**
  - Assert `GET {{base_url}}/book/test-token` returns 200 with `text/html` (testing deep-link SPA routing).
- **`03_booking_lifecycle.hurl` (or new `08_secret_link_booking.hurl`):**
  - Create booking via `POST /api/bookings` with `X-Secret-Token` header and no `setup_intent_id`.
  - Assert 201 response with `confirmed` status.
  - Verify conflict rejection on the same slot without card.
- **`04_admin_bookings_charges.hurl`:**
  - Charge attempt on a free booking returns 400 (not 500).
- **`07_email_flow.hurl`:**
  - Assert Mailpit receives free booking confirmation with "Studio booking - no payment required" copy.

### 3. Playwright E2E Suites (`tests/e2e/tests/`)
The repository now runs browser tests via `make test-e2e`. We must extend:
- **`spa-navigation.spec.ts`:**
  - Assert `/book/invalid-token` loads the page and displays the "invalid or expired link" message.
- **`public-booking.spec.ts` (or new `secret-booking.spec.ts`):**
  - Navigate to `/book/{valid_token}`.
  - Assert header says "Studio Artist" / "No payment required".
  - Assert no Stripe card element is present.
  - Pick a slot and complete booking; assert success screen without payment wording.
- **`admin.spec.ts`:**
  - In Settings -> Booking rules: toggle secret link on, copy link, rotate token.
  - In Bookings table: assert artist booking displays the "Free · Studio booking" badge and **no visible/enabled Charge action**.

### 4. Contract Documentation (`tests/CONTRACT.md`)
- Update `tests/CONTRACT.md` documenting the new assertions, authority, and status codes for:
  - `GET /api/secret-link/verify` (200 + `valid` boolean; uniform for valid/invalid)
  - `POST /api/admin/secret-link/rotate`
  - `POST /api/admin/bookings/{id}/charge` → 400 on free bookings (authority: `via_secret_link` column + nil-PM guard)
  - `GET /api/settings/public` → no `secret_link` key ever (privacy pin)
  - Secret token parameters on availability and booking create endpoints.

### 5. Docs & Spec (`api/openapi.yaml`, `README.md`)
- Repo convention (every feature commit touches both, e.g. `f8b0e88`): add the new endpoints/params to `api/openapi.yaml` and document the feature in `README.md` (rotate semantics: old links die immediately).

