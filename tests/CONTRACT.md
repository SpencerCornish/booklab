# Test contract — what the suites assert and why

This document spells out every behavioral expectation the functional suites
encode, and where each expectation's authority comes from. Its purpose is to
let you validate the contract: **if an expectation here looks wrong to you, the
test is wrong** (or it caught something worth discussing).

## How expectations were derived (anti-enshrinement)

A black-box test can silently enshrine bugs by asserting whatever the server
currently does. These suites guard against that in four ways:

1. **Every expectation is anchored to an authority outside the test** — a
   database constraint (the strongest contract in this app), a code constant
   with documented intent, an HTTP/security semantic, or a user-visible design
   decision. The tables below cite the authority for each expectation.
2. **When a test met a bug, the app changed, not the test.** The availability
   suite initially asserted "an all-day closure closes the day" and *failed*
   against master (closures were invisible under the default America/Denver
   timezone). If the suites merely enshrined behavior, that test would have
   been written to expect `is_closed: false`. Instead it caught a production
   bug — fixed in `internal/db/queries/closures.sql`, `internal/api/closures.go`,
   `internal/api/public.go`, with regression tests.
3. **Assertions target observable API/UI behavior, not internals** — no
   implementation details, no stack traces, no internal IDs beyond public ones.
4. **Judgment calls are flagged** (§ "Flagged for your review") — places where
   the contract is genuinely ambiguous and an interpretation was chosen. Those
   are the lines to read most carefully.

## Authority map

| Authority | Used for |
|---|---|
| DB schema/migrations (`internal/db/migrations/`) | double-booking exclusion constraint, settings defaults, status enum, unique setup-intent index |
| Code constants with documented intent | rate limits (10 fails/15 min), max charge ($200), min password (8), session duration (24 h), CSRF cookie names |
| HTTP + security semantics | status codes, `HttpOnly`/`Secure`/`SameSite` cookie attributes, 401 vs 403 |
| Product-visible design (comments in handlers/components) | which bookings can be cancelled, session revocation on password change, probe-path 404s |
| User-visible copy (email subjects) | email assertions — **pinned to copy; changing copy updates tests** |

> One deliberate *environment* pin (not a business contract): the test app runs
> with `TZ=UTC` (matching production containers), so response timestamps assert
> UTC instants like `2030-03-15T16:00:00Z`. The business contract is "the same
> instant, valid RFC 3339"; the suite pins the format so serialization changes
> surface loudly rather than silently.

---

## `00_health.hurl` — public surface & settings defaults

| Behavior | Expectation | Authority |
|---|---|---|
| `GET /api/health` | 200 `{"status":"ok"}`, unauthenticated, JSON | New endpoint added in this branch (cheap liveness for harnesses) |
| `GET /api/settings/public` | Exact defaults: `The Space`, 1500¢/hr, `usd`, `America/Denver`, 09:00–21:00, min 1 h, max 8 h, min-lead 0 | Migration `001_initial.up.sql` seed defaults + `013` (min_lead 0). Defaults are product contract; changing them should be a conscious act |
| `GET /api/terms`, `/api/privacy` | 200, `content` key exists | Routes exist; content is data-driven (default empty → existence, not copy) |
| `GET /api/nope` | 404 | Unknown `/api/*` must not fall through to the SPA catch-all |

## `01_admin_auth.hurl` — sessions, CSRF, rate limiting

| Behavior | Expectation | Authority |
|---|---|---|
| Admin endpoint without session | 401 | `requireAdmin` |
| Wrong password | 401 | bcrypt compare |
| Correct login | 200 + `booklab_session` cookie with `HttpOnly`, `Secure`, `SameSite=Lax` | `setSessionCookie` — **security attributes are requirements, pinned on purpose** |
| First admin request | Sets `booklab_csrf` (JS-readable, not HttpOnly) | `csrfProtect` bootstrap; the SPA must read it to mirror the header |
| Mutating request without / with wrong `X-CSRF-Token` | 403 | Double-submit constant-time compare in `csrfProtect` |
| With correct token | Request succeeds (201) | — |
| Logout | 204; subsequent admin call 401 | Server-side session deletion (not just cookie clear) |
| Failed logins | 9 more → 401 each; the **10th** → 429 **even with correct credentials** | `loginRateLimitMax = 10` / window 15 min per IP *and* username; the check precedes credential verification by design |

## `02_availability.hurl` — slot computation & closures

| Behavior | Expectation | Authority |
|---|---|---|
| Open future day | 12 hourly slots (09:00–21:00 Denver), all available | Settings bookable window → slot loop |
| `GET /api/availability/days` | Contains tomorrow/day-after (runner-computed); invalid `days` → 400; window capped at 90 | PR #6 endpoint; anchored to "now" by design (feeds the 60-day picker) |
| Booking 10:00–12:00 | Exactly slots 10:00 and 11:00 become unavailable | Overlap check (`slot.start < b.end && slot.end > b.start`) |
| **All-day closure** | `is_closed: true`, reason echoed, zero slots; day also disappears from the day-strip | **Product intent: closures close days.** This assertion failed against master and produced the timezone bug fix — the flagship anti-enshrinement case |
| Near-term closure (tomorrow) | Same behavior at DST-relevant near dates | Guards the fix across calendar arithmetic |
| Partial closure 12:00–14:00 | Exactly slots 12:00 + 13:00 blocked | `slotOverlapsPartialClosure` |
| Missing/malformed `date` | 400 | Handler validation |

## `03_booking_lifecycle.hurl` — the core business flow

| Behavior | Expectation | Authority |
|---|---|---|
| prepare → create | 201, `status: confirmed`, fields echoed | `CreateBooking` |
| Replay same `setup_intent_id` | 201 returning the *same* booking | Idempotency by unique index (migration `011`) + early return |
| prepare on a taken/overlapping slot | 409 | `ensureSlotAvailable` — ⚑ judgment call (see review list) |
| create: valid intent, conflicting slot | 409 | **DB exclusion constraint `bookings_no_overlap`** — the deepest contract in the app, exercised directly |
| Adjacent (touching) slot | 201 | Half-open `tstzrange` semantics |
| prepare/create validation | past start, `end ≤ start`, missing fields, empty body, 11 h (max 8) → 400 | `validateBookingCreate` |
| Unknown setup intent at create | 400 "card not confirmed" | `GetPaymentMethodFromSetupIntent` error path; the fake mirrors real-Stripe rejection ⚑ |
| `GET /api/bookings/{token}` | 200 for real token; 400 non-UUID; 404 unknown UUID | `uuid.Parse` + lookup |
| Cancel confirmed booking | 200 `cancelled` | `CancelBooking … WHERE status='confirmed'` |
| Cancel again / unknown token | 409 | Only confirmed bookings are cancellable — ⚑ judgment call |
| Rebook freed slot | 201 | Exclusion constraint ignores cancelled rows |

## `04_admin_bookings_charges.hurl` — list, edit, charge

| Behavior | Expectation | Authority |
|---|---|---|
| List shows booking with admin-only fields (setup-intent, PM ids) | 200, fields present | `bookingToAdmin` |
| Filters `?status=`, `?date=` | Correct subsets; malformed date → 400 | `ListBookingsParams` |
| Invalid `?status=bogus` | **500** | ⚑ **Flagged — this codifies observed behavior that is arguably wrong** (should likely be 400). Kept as-is so a future fix is a visible, deliberate diff |
| PATCH end_time / status=completed (PM on file) | 200, echoed | `UpdateBookingEndTime/Status` |
| Charge, no amount | Computed: hours × 1500 (3 h → 4500), `status: charged`, `pi_fake_*`, receipt URL | `handleAdminChargeBooking` computed amount |
| Second charge / charge charged booking | 409 | Claim-based idempotency (`ClaimBookingForCharge`) |
| Charge unknown booking | 404 | — |
| `amount_cents: 0` / `25000` | 400 **and booking remains chargeable afterwards** | `maxChargeAmountCents = 20000` + explicit revert-to-completed logic (intent documented in handler) |

## `05_admin_settings_users_closures.hurl` — admin CRUD

| Behavior | Expectation | Authority |
|---|---|---|
| Partial settings PUT | Reflected in admin **and** public settings | Public/admin consistency is the point of the split |
| Invalid `bookable_start` / negative `min_lead_hours` / restriction `end < start` | 400 | Handler validation |
| `new_customer_restriction` | Valid window round-trips; `null` clears | `parseNewCustomerRestriction` (null vs absent distinction documented there) |
| Users | create → 201; duplicate → 409; short password → 400; delete self → 400; delete unknown → 404; delete other → 204 | Unique constraint, `minAdminPasswordLength`, self/last-admin guards |
| Password change | 204; **all old sessions revoked**; new session issued; old password rejected | Documented intent in `handleAdminChangePassword` (stolen cookie must not outlive password change) |
| Closures CRUD | Defaults to all-day; `end < start` → 400; partial without times → 400; update/delete round-trip | `parseClosureUpsert` |

## `06_spa_routing.hurl` — embedded SPA & probe defense

| Behavior | Expectation | Authority |
|---|---|---|
| `/` | 200 HTML (embedded bundle) | `handleSPA` + embed |
| Hashed asset | Captured from `index.html`, fetched with JS content type | Vite output contract |
| Client routes (`/admin`, `/booking/x`, `/cancel/x`, `/terms`, `/privacy`) | 200 `index.html` | `isSPARoute` ↔ `App.tsx` route table |
| Scanner probes (`/.env`, `/.git/config`, `/wp-login.php`, `/phpmyadmin/`, `/actuator/health`, `/*.sql`, `/*.bak`, `/debug/*`) | 404, never the SPA | `isProbePath` — **security intent documented in `spa.go`; these are requirements** |
| Unknown path / unknown `.js` | 404 | Non-SPA paths don't get `index.html` |

## `07_email_flow.hurl` — Mailpit-verified delivery

| Behavior | Expectation | Authority |
|---|---|---|
| Booking created | Email arrives (async, retried) with subject `Booking Confirmed – The Space` | `SendConfirmation`; **copy pinned — changing subjects updates tests** |
| Confirmation body | Contains `/booking/{token}` and `/cancel/{token}` **for this booking's token** | `ConfirmationData` URLs — asserts API token == email link integrity |
| Cancel / charge | Corresponding emails with correct subjects; receipt body shows the charged amount (`$30.00`) | `SendCancellation`, `SendReceipt` |
| Exactly 4 messages for the recipient | No unexpected email (e.g. no reminder for a far-future booking) | Reminder only fires within `reminder_hours_before` (24 h) |

## E2E specs (`tests/e2e/tests/`)

| Spec | Encodes |
|---|---|
| `spa-navigation` | Public header derived from settings; `/admin` redirects unauthenticated users to login; unknown tokens render error states; unknown paths are **server** 404s (matching Hurl 06); a real booking renders on its view page |
| `public-booking` | Slot grid reflects the bookable window; **closed days are disabled with `title="No availability"`** (PR #6 picker — the user-facing signal that replaced a reachable banner); selecting 10:00 + 11:00 extends *through* 12:00 (2 hr) with `$30.00` estimated total; terms checkbox gates submit; in a no-key build submit stays disabled (fail-safe: booking cannot complete without payment) |
| `booking-cancel` | Cancel via link is terminal ("No charge will be made"); the freed slot is bookable again (API cross-check) |
| `admin` | Bad login shows an error; bookings list shows seeded data (visible-layout aware); closures CRUD with `confirm()` dialog; settings round-trip on the `/general` subpage **and reflects on the public page**, then restores; users create/delete; sign out truly revokes (next `/admin` nav bounces to login) |
| `payment` | Full Stripe.js card flow (4242…, test card) → "You're booked!" + confirmation email. **Safety: skips without keys; aborts on any non-`sk_test_`/`pk_test_` key — cannot touch live Stripe** |

## Go regression tests added in `internal/api/public_test.go`

- All-day closure under `America/Denver`: day is closed, neighbor day still open with 12 slots.
- Partial closure under `America/Denver`: exactly the two covered slots blocked.
- (Plus `TestHandleHealth` for the new endpoint.)

---

## Flagged for your review ⚑

These are the lines where I chose an interpretation — read these first:

1. **`prepare` on a taken slot → 409.** I initially wrote the test expecting 200 and corrected the *test* after observing the design (prepare validates availability up-front so the Stripe handoff never happens for dead slots). If you'd rather have prepare always 200, that's a product decision to make consciously.
2. **`GET /api/admin/bookings?status=bogus` → 500.** Codified observed behavior; arguably should be 400. I pinned it so a fix shows up as a deliberate test change, not silence.
3. **Cancel of a charged booking → 409.** Follows the `WHERE status='confirmed'` rule. If admins should be able to cancel charged bookings (with refund flow), the test needs updating with the feature.
4. **Unknown setup intent → 400.** The fake rejects IDs it didn't issue, mirroring real Stripe. If you'd prefer the fake to be more permissive, say so.
5. **Email subjects pinned to copy** (`Booking Confirmed – The Space`, etc.). Copy edits will fail tests loudly by design — update both together.
6. **Settings defaults pinned exactly** (resource name, rate, hours). Changing defaults should be a conscious act; the suite makes you do it twice.
7. **`TZ=UTC` response format.** Environment pin, not business semantics (see note above).