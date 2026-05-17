---
name: BookLab Security Fixes
overview: "Implement priority-ordered security fixes across BookLab: prevent double-charges via atomic DB claims and Stripe idempotency, harden admin auth with server-side sessions and rate limiting, add CSRF protection, fix CORS, enforce server-side booking policy, and apply input/error hygiene."
todos:
  - id: p0-db-claim
    content: Add 003 migration for 'charging' status + ClaimBookingForCharge/ClaimBookingForAutoCharge queries in queries.go and models.go
    status: pending
  - id: p0-stripe-idempotency
    content: Add idempotencyKey param to ChargePaymentMethod in stripe.go
    status: pending
  - id: p0-admin-charge
    content: Rewrite handleAdminChargeBooking to use claim pattern, idempotency key, 409 on duplicate, amount_cents validation
    status: pending
  - id: p0-scheduler
    content: Rewrite autoChargeCompleted to use claim-one-at-a-time loop with ClaimBookingForAutoCharge
    status: pending
  - id: p1-sessions-db
    content: Add 004 migration for admin_sessions + login_attempts tables, add session/rate-limit query methods
    status: pending
  - id: p1-auth-rewrite
    content: "Rewrite middleware.go: DB-backed session create/validate/delete, remove HMAC token code"
    status: pending
  - id: p1-login-rate-limit
    content: Add rate limiting to handleAdminLogin, rewrite login/logout to use DB sessions
    status: pending
  - id: p1-csrf
    content: Add csrfProtect middleware, wire into admin routes, update frontend api.ts with X-CSRF-Token header
    status: pending
  - id: p2-cors
    content: Add CORS_ALLOWED_ORIGINS config, replace wildcard with explicit allowlist in server.go
    status: pending
  - id: p2-booking-policy
    content: Add server-side booking policy validation in handleCreateBooking (hours, window, closures, past-time)
    status: pending
  - id: p3-body-limits
    content: Add MaxBytesReader to readJSON, return 413 on oversized bodies
    status: pending
  - id: p3-error-sanitize
    content: Remove raw Stripe error from charge response, log internally
    status: pending
  - id: p3-email-headers
    content: Add CRLF sanitization to email header fields in buildMessage
    status: pending
  - id: tests
    content: Add tests for double-charge, race conditions, auth, CSRF, CORS, booking policy, body limits, email headers
    status: pending
isProject: false
---

# BookLab Security Fix Plan

This plan covers 6 priority-ordered steps. Each step lists the exact files to change, what to change, and the key code patterns to use.

---

## Step 1 -- P0: Prevent duplicate/unsafe charges

The core bug: `handleAdminChargeBooking` ([internal/api/admin.go](internal/api/admin.go) line 164) fetches the booking via `GetBookingByID` (a plain SELECT), performs the Stripe charge, then calls `UpdateBookingCharged` -- a non-conditional UPDATE. Nothing prevents two concurrent callers from both passing the initial read and charging the same booking. The scheduler path in `autoChargeCompleted` ([internal/scheduler/scheduler.go](internal/scheduler/scheduler.go) line 80) has the same race.

### 1a. Add atomic claim query in DB layer

In [internal/db/queries.go](internal/db/queries.go), add a new method `ClaimBookingForCharge`:

```go
func (q *Queries) ClaimBookingForCharge(ctx context.Context, id int32) (*Booking, error) {
    row := q.pool.QueryRow(ctx, `
        UPDATE bookings SET status = 'charging'
        WHERE id = $1
          AND status IN ('completed')
          AND stripe_payment_intent_id IS NULL
          AND amount_cents IS NULL
        RETURNING `+bookingColumns,
        id,
    )
    return scanBooking(row)
}
```

This is an atomic conditional UPDATE. If two callers race, only one gets a row back; the other gets `pgx.ErrNoRows`. No transaction or `FOR UPDATE` needed since the UPDATE itself is atomic.

- Add `'charging'` to the `booking_status` enum via a new migration `003_charging_status.up.sql` in [internal/db/migrations/](internal/db/migrations/).
- Add `BookingStatusCharging BookingStatus = "charging"` to [internal/db/models.go](internal/db/models.go).

Also add `ClaimBookingForAutoCharge` for the scheduler path (selects first unclaimed completed booking and claims it):

```go
func (q *Queries) ClaimBookingForAutoCharge(ctx context.Context) (*Booking, error) {
    row := q.pool.QueryRow(ctx, `
        UPDATE bookings SET status = 'charging'
        WHERE id = (
            SELECT id FROM bookings
            WHERE status = 'completed'
              AND completed_at IS NOT NULL
              AND completed_at < NOW() - INTERVAL '24 hours'
              AND stripe_payment_method_id IS NOT NULL
              AND amount_cents IS NULL
            ORDER BY completed_at
            LIMIT 1
            FOR UPDATE SKIP LOCKED
        )
        RETURNING `+bookingColumns,
    )
    return scanBooking(row)
}
```

### 1b. Add Stripe idempotency key support

In [internal/stripe/stripe.go](internal/stripe/stripe.go), modify `ChargePaymentMethod` to accept an `idempotencyKey string` parameter. Set it on the Stripe params:

```go
func (s *Service) ChargePaymentMethod(pmID string, amountCents int64, currency, description, idempotencyKey string) (piID, receiptURL string, err error) {
    // ...existing code...
    piParams.IdempotencyKey = stripe.String(idempotencyKey)
    // ...
}
```

Callers generate a deterministic key: `fmt.Sprintf("charge-booking-%d", bookingID)`.

### 1c. Rewrite `handleAdminChargeBooking`

In [internal/api/admin.go](internal/api/admin.go) line 164:

1. Replace `GetBookingByID` with `ClaimBookingForCharge(ctx, id)`.
2. If `pgx.ErrNoRows`, return `409 Conflict` ("booking already charged or not eligible").
3. Pass idempotency key to `ChargePaymentMethod`.
4. On Stripe failure, revert the claim: `UpdateBookingStatus(ctx, id, BookingStatusCompleted)`.
5. Remove the raw Stripe error from the response (see Step 6).

### 1d. Rewrite `autoChargeCompleted` in scheduler

In [internal/scheduler/scheduler.go](internal/scheduler/scheduler.go) line 80:

1. Replace `ListBookingsDueAutoCharge` loop with a claim-one-at-a-time loop using `ClaimBookingForAutoCharge`.
2. Call in a loop until `ErrNoRows` (no more claimable bookings).
3. Pass idempotency key to Stripe.
4. On failure, revert claim back to `completed`.

### 1e. Add `amount_cents` validation

In `handleAdminChargeBooking`, validate the override amount: `amount_cents` must be > 0 and <= a sane max (e.g. 20,000 cents / $200). Reject otherwise with 400.

---

## Step 2 -- P1: Harden admin authentication/session security

Currently tokens are stateless HMAC strings with format `username:expiry:sig` ([internal/api/middleware.go](internal/api/middleware.go) line 14). No server-side revocation, no rate limiting, and username containing `:` would break parsing.

### 2a. Add DB-backed sessions

New migration `004_sessions_and_rate_limits.up.sql`:

```sql
CREATE TABLE admin_sessions (
    id         TEXT PRIMARY KEY,  -- random token
    username   TEXT NOT NULL REFERENCES admin_users(username),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE login_attempts (
    id         SERIAL PRIMARY KEY,
    username   TEXT NOT NULL,
    ip_address TEXT NOT NULL,
    success    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_login_attempts_ip ON login_attempts(ip_address, created_at);
CREATE INDEX idx_login_attempts_user ON login_attempts(username, created_at);
```

Add query methods in [internal/db/queries.go](internal/db/queries.go):

- `CreateSession(ctx, id, username, expiresAt)` -- INSERT.
- `GetSession(ctx, id)` -- SELECT with `expires_at > NOW()` check.
- `DeleteSession(ctx, id)` -- for logout.
- `DeleteSessionsByUsername(ctx, username)` -- for revoke-all.
- `RecordLoginAttempt(ctx, username, ip, success)`.
- `CountRecentFailedAttempts(ctx, username, ip, window)` -- counts failures in time window.

### 2b. Rewrite token generation and validation

In [internal/api/middleware.go](internal/api/middleware.go):

- `newSessionToken` becomes: generate `crypto/rand` hex token, `CreateSession` in DB, return token string.
- `validateSessionToken` becomes: `GetSession` from DB. If not found or expired, return false.
- `requireAdmin` middleware: on valid session, also verify the admin user still exists via `GetAdminByUsername`.
- Delete the old HMAC `sign()` function and delimiter-based parsing.

### 2c. Rewrite login handler with rate limiting

In `handleAdminLogin` ([internal/api/admin.go](internal/api/admin.go) line 18):

1. Extract client IP via `r.RemoteAddr` (already behind `middleware.RealIP`).
2. Call `CountRecentFailedAttempts` for both IP and username (window: 15 min).
3. If either exceeds threshold (e.g. 10 failures), return 429 Too Many Requests.
4. On success: `RecordLoginAttempt(success=true)`, create session, set cookie.
5. On failure: `RecordLoginAttempt(success=false)`, return 401.

### 2d. Rewrite logout handler

In `handleAdminLogout` ([internal/api/admin.go](internal/api/admin.go) line 56): extract session ID from cookie, call `DeleteSession`, then clear the cookie.

---

## Step 3 -- P1: CSRF protection for admin mutations

Cookie-based auth with browser requests requires CSRF defense. The admin UI is a same-origin SPA, so a double-submit cookie pattern is the lightest approach.

### 3a. Add CSRF middleware

In [internal/api/middleware.go](internal/api/middleware.go), add a `csrfProtect` middleware:

- On any request: check for a `booklab_csrf` cookie. If absent, generate a random token, set it as a `SameSite=Strict` non-HttpOnly cookie (so JS can read it).
- On mutating methods (POST/PUT/PATCH/DELETE): compare the `X-CSRF-Token` header value to the `booklab_csrf` cookie value. If missing or mismatch, return 403.
- Exempt `/api/admin/login` from the header check (it's the bootstrap endpoint).

### 3b. Wire middleware into router

In [internal/api/server.go](internal/api/server.go) line 56, add `r.Use(s.csrfProtect)` inside the admin group, after `requireAdmin`.

### 3c. Update frontend API client

In [web/src/lib/api.ts](web/src/lib/api.ts), modify the `request` function to read the `booklab_csrf` cookie and add it as the `X-CSRF-Token` header on mutating requests:

```typescript
function getCsrfToken(): string | undefined {
  const match = document.cookie.match(/booklab_csrf=([^;]+)/)
  return match?.[1]
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  const method = options?.method?.toUpperCase() ?? 'GET'
  if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(method)) {
    const csrf = getCsrfToken()
    if (csrf) headers['X-CSRF-Token'] = csrf
  }
  // ... rest unchanged
}
```

---

## Step 4 -- P2: Fix CORS and enforce server-side booking policy

### 4a. Fix CORS configuration

In [internal/api/server.go](internal/api/server.go) line 36:

1. Add `AllowedOrigins` config field to [internal/config/config.go](internal/config/config.go): `CORSAllowedOrigins string \`envconfig:"CORS_ALLOWED_ORIGINS" default:""`.
2. In the CORS handler, replace `[]string{"*"}` with the parsed origin list. If empty/unset, default to `[cfg.AppURL]`.
3. Add `X-CSRF-Token` to `AllowedHeaders`.

### 4b. Enforce server-side booking policy

In `handleCreateBooking` ([internal/api/public.go](internal/api/public.go) line 128), after parsing the request and loading settings, add validation:

1. Load timezone from settings, convert booking times to local.
2. Reject if `start_time` is in the past.
3. Reject if duration < `min_hours` or > `max_hours`.
4. Reject if booking falls outside `bookable_start`/`bookable_end` window.
5. Reject if the booking date overlaps any closure (query `ListClosuresInRange`).

---

## Step 5 -- P3: Input/error hygiene

### 5a. Request body size limits

In [internal/api/helpers.go](internal/api/helpers.go), wrap `readJSON` with `http.MaxBytesReader`:

```go
func readJSON(r *http.Request, v any) error {
    r.Body = http.MaxBytesReader(nil, r.Body, 1<<20) // 1 MB
    dec := json.NewDecoder(r.Body)
    dec.DisallowUnknownFields()
    return dec.Decode(v)
}
```

Also add a check: if the error is `*http.MaxBytesError`, the caller should return 413. Update `readJSON` callers or make `readJSON` return a sentinel.

### 5b. Sanitize Stripe error in charge response

In `handleAdminChargeBooking` ([internal/api/admin.go](internal/api/admin.go) line 211): replace `"charge failed: "+err.Error()` with a generic `"payment failed"` message. Log the detailed error server-side with `log.Printf`.

### 5c. Email header injection prevention

In [internal/email/email.go](internal/email/email.go), in `buildMessage` (line 163) and the `send` method: validate/sanitize the `to`, `subject`, and `from` fields. Reject or strip any `\r` or `\n` characters:

```go
func sanitizeHeader(s string) string {
    return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}
```

Apply `sanitizeHeader` to all header values in `buildMessage`.

---

## Step 6 -- Tests and verification

- Add `internal/api/admin_test.go` and `internal/scheduler/scheduler_test.go`.
- Charge idempotency tests: call `handleAdminChargeBooking` twice on the same booking; assert second returns 409 and no second Stripe call.
- Scheduler race test: run two `autoChargeCompleted` goroutines concurrently; assert exactly one charge per booking.
- Auth tests: verify session revocation works, brute-force lockout triggers at threshold.
- CSRF tests: verify mutating request without `X-CSRF-Token` returns 403; with token succeeds.
- CORS test: verify non-allowlisted origin gets no `Access-Control-Allow-Origin` header.
- Booking policy tests: out-of-hours, over-max-duration, closure-day, past-time all rejected.
- Body limit test: 2 MB body returns 413.
- Email header test: `\r\n` in subject/to is stripped.

---

## Files changed summary


| Priority | Files                                                                                                                                                                          |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| P0       | `internal/db/queries.go`, `internal/db/models.go`, `internal/db/migrations/003_*.sql`, `internal/stripe/stripe.go`, `internal/api/admin.go`, `internal/scheduler/scheduler.go` |
| P1-auth  | `internal/db/queries.go`, `internal/db/migrations/004_*.sql`, `internal/api/middleware.go`, `internal/api/admin.go`                                                            |
| P1-csrf  | `internal/api/middleware.go`, `internal/api/server.go`, `web/src/lib/api.ts`                                                                                                   |
| P2       | `internal/api/server.go`, `internal/config/config.go`, `internal/api/public.go`                                                                                                |
| P3       | `internal/api/helpers.go`, `internal/api/admin.go`, `internal/email/email.go`                                                                                                  |
| Tests    | `internal/api/admin_test.go`, `internal/scheduler/scheduler_test.go`                                                                                                           |


