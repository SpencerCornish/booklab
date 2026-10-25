# BookLab Functional Testing Plan

Functional (black-box) test suites that exercise the **real binary**, **real database**,
**real SPA bundle**, and **real email sink** — complementing the existing in-process
`httptest` suites in `internal/api/*_test.go`.

**Agreed decisions:**
- **Stripe**: hermetic fake by default (`STRIPE_FAKE=1` behind the existing
  `stripe.Client` interface) + one real-key payment E2E test, hard-gated to
  *test-mode* keys only (see §5 "Real-Stripe safety guardrails").
- **Platform**: GitHub Actions is the primary target; the same `make` targets run locally.

## 1. Goals

- **Out-of-process**: run against a built `booklab` binary (embedded SPA included), not `httptest`.
- **Hermetic by default**: everything the suites need is spun up by Compose; no external accounts required.
- **Deterministic**: fresh DB state per suite, fixed future dates, unique emails.
- **One command per suite**, plus CI wiring that reuses the same commands.

## 2. What exists today (baseline)

| Area | Status |
|---|---|
| Go unit/integration tests | `internal/api/{admin,public,spa}_test.go`, `internal/scheduler/scheduler_test.go` — httptest full server + real Postgres (`TEST_DATABASE_URL`) + `mockStripe` implementing `internal/stripe/client.go` |
| Frontend tests | None (`web/package.json` has no test runner) |
| CI (`.github/workflows/ci.yml`) | `go` job (vet + tests w/ Postgres service), `web` job (tsc + build). No docker build, no functional/E2E tests |
| HTTP surface | 24 API routes registered in `internal/api/server.go:61-113` + SPA catch-all (`internal/api/spa.go`) |
| Auth | Cookie session `booklab_session` + double-submit CSRF (`booklab_csrf` cookie ↔ `X-CSRF-Token` header), see `internal/api/middleware.go` |
| Email | Raw SMTP via `internal/email` — no interface seam; Mailpit already wired in `docker-compose.yml` |
| Stripe | `stripe.Client` interface (`internal/stripe/client.go`) already faked in-process by tests |
| Health endpoint | **None** — readiness must be polled via `GET /api/settings/public` today |

## 3. Shared test infrastructure

New tree:

```
tests/
  compose/docker-compose.test.yml   # isolated pg + mailpit (+ nothing else)
  scripts/
    stack-up.sh stack-down.sh       # compose project mgmt (booklab-functest)
    reset-db.sh                     # TRUNCATE state tables, keep admin_users
    run-app.sh                      # env -> build/run binary, wait ready
  hurl/
    vars/                           # base_url, admin creds, dates, emails
    suites/
      00_health.hurl
      01_admin_auth.hurl
      02_availability.hurl
      03_booking_lifecycle.hurl
      04_admin_bookings_charges.hurl
      05_admin_settings_users_closures.hurl
      06_spa_routing.hurl
      07_email_flow.hurl
  e2e/
    package.json                    # Playwright in its OWN pnpm package (see note)
    playwright.config.ts
    tests/
      public-booking.spec.ts
      booking-cancel.spec.ts
      admin.spec.ts
      spa-navigation.spec.ts
      payment.spec.ts               # real Stripe test keys only (skipped w/o keys)
    helpers/
      mailpit.ts admin-session.ts fixtures.ts
```

> **Why Playwright lives outside `web/`**: the Dockerfile (`Dockerfile:6`) runs
> `pnpm install --frozen-lockfile` in `web/` at image build time; adding Playwright
> (≈100 MB of browser tooling) there would bloat every production image build.

Shared conventions:

- **Ports** (all non-default, so dev servers never collide): app `8090`, Postgres `5433`, Mailpit SMTP `1026` / UI `8027`.
- **Compose project name** `booklab-functest` with its own Postgres volume — dev data (`pgdata` from `docker-compose.yml`) is never touched. Teardown is `docker compose -p booklab-functest down -v`.
- **DB reset between Hurl files**: `tests/scripts/reset-db.sh` runs
  `TRUNCATE bookings, closures, admin_sessions, login_attempts;` — keeps `admin_users`
  (bootstrap admin is only created at boot, `cmd/server/main.go:42-46`) and keeps `settings`.
- **Dates**: all fixtures target fixed future dates (e.g. 2030-xx-xx) and explicit
  settings timezone, so tests never collide with "now" logic (min-lead, bookable window)
  or with the 30-minute background scheduler tick (`internal/scheduler`).
- **URLs use `localhost`**, never `127.0.0.1`/container IPs — the `Secure` session cookie
  is accepted by Chromium on `http://localhost` but not on arbitrary hosts.
- **Recommended small app change — `GET /api/health`**: unauthenticated `{"status":"ok"}`
  in `internal/api/server.go`. Makes readiness polling trivial for scripts, Compose
  healthchecks, and CI. Without it, scripts poll `GET /api/settings/public`.

## 4. Suite 1 — Hurl API functional tests

Black-box HTTP tests against the running binary. Focus on what the in-process Go tests
**can't** see: real wire behavior of the binary, headers, cookies, CORS in real config,
SPA file serving, and cross-cutting flows. Deliberately *not* re-testing every validation
branch that `internal/api/public_test.go` already covers.

### Coverage map

| File | Exercises (route refs) | Key assertions |
|---|---|---|
| `00_health.hurl` | `GET /api/health` (new), `GET /api/settings/public`, `GET /api/terms`, `/api/privacy` | 200s, JSON shapes, `Content-Type` |
| `01_admin_auth.hurl` | `POST /api/admin/login`, `POST /api/admin/logout` (`internal/api/admin.go:37,96`) | login sets `booklab_session` (HttpOnly) + `booklab_csrf` cookies; bad password → 401; missing/incorrect `X-CSRF-Token` → 403; admin routes without session → 401; logout revokes (next admin call → 401); rate limit: 10 bad logins → 429 (`admin.go:24-25`) |
| `02_availability.hurl` | `GET /api/availability?date=…` (`public.go:72`) | slot grid for an open day; closures subtract slots (`05_admin_*` creates the closure first); min-lead behavior for near-term dates |
| `03_booking_lifecycle.hurl` | `POST /api/bookings/prepare` → `POST /api/bookings` → `GET /api/bookings/{token}` → `POST /api/bookings/{token}/cancel` (`public.go:167,228,314,382`) | 201 + token capture; double-book same slot rejected (DB exclusion constraint); duplicate `setup_intent_id` idempotency; cancel frees slot; cancelled booking view |
| `04_admin_bookings_charges.hurl` | `GET/PATCH /api/admin/bookings(/{id})`, `POST /api/admin/bookings/{id}/charge` (`admin.go:284,313,399`) | list pagination/filtering; PATCH transitions; charge success → `charged`; second charge → 409 (idempotent); charge without saved PM rejected |
| `05_admin_settings_users_closures.hurl` | settings, users, closures endpoints (`admin.go:526,535,120-185,615-700`) | settings round-trip; create/delete extra admin user; change own password then re-login; closures CRUD; new-customer-restriction flag reflected in public settings |
| `06_spa_routing.hurl` | `handleSPA` (`server.go:115`) + `internal/api/spa.go` | `/` serves `index.html`; `/assets/*` served with immutable caching; `/admin`, `/booking/x`, `/cancel/x`, `/terms` → `index.html`; probe paths (`/.env`, `/wp-login.php`, `*.sql`…) → 404; unknown path → 404 |
| `07_email_flow.hurl` | booking create/cancel + Mailpit REST | confirmation email exists in Mailpit (`GET /v1/messages` on `:8027`); subject/recipient correct; body contains `/booking/{token}` and `/cancel/{token}` links; cancellation email after cancel |

### Mechanics

- **Auth**: Hurl persists cookies between entries in a run. First mutating request
  receives the `booklab_csrf` cookie (`middleware.go:88-129`); capture it from `Set-Cookie`
  and echo it as `X-CSRF-Token` thereafter. (Verify exact capture syntax against the Hurl
  version we pin; fallback is a `regex` capture on the `Set-Cookie` header.)
- **Variables**: `tests/hurl/vars/functional.env` — `base_url`, `admin_user`, `admin_pass`,
  `booking_date`, `customer_email`, `mailpit_url`. Per-run overrides via `--variable`.
- **Hermetic Stripe (small app change)**: `cmd/server/main.go` currently hardwires
  `stripesvc.New(cfg.StripeSecretKey, …)`. Add a fake mode selected by env:

  ```go
  var stripeService stripesvc.Client = stripesvc.New(cfg.StripeSecretKey, logger)
  if cfg.StripeFake { stripeService = stripesvc.NewFake() }
  ```

  `NewFake()` (~50 lines in `internal/stripe`) returns deterministic IDs
  (`seti_fake_*`, `pm_fake_*`, `pi_fake_*`, a fake receipt URL) — the `stripe.Client`
  interface makes this trivial and both `api.Server` and `scheduler` consume the interface
  already. With `STRIPE_FAKE=1`, the full booking lifecycle in `03_booking_lifecycle.hurl`
  works with **zero network access**.
- **Orchestration** (`Makefile`):

  ```make
  test-func-api:   ## Hurl suite against the built binary + compose stack
  	$(MAKE) stack-up reset-db build-api
  	tests/scripts/run-app.sh --env STRIPE_FAKE=1 --port 8090   # waits for /api/health
  	hurl --test --variables-file tests/hurl/vars/functional.env \
  	     --report-junit test-results/hurl.xml tests/hurl/suites/*.hurl
  	$(MAKE) stack-down
  ```

  Each Hurl file runs with a DB reset between files for determinism.
- **Local install**: add `hurl` to `.tool-versions` (asdf) to match repo conventions;
  CI uses the `ghcr.io/orange-opensource/hurl` container image instead.

## 5. Suite 2 — Playwright E2E (stand up the stack, cycle the UI)

`@playwright/test` in its own package (`tests/e2e`), Chromium-only for speed.

### Stack shape under test

Same stack as Suite 1 (compose Postgres + Mailpit, binary on `:8090` with **embedded**
SPA built by `make build-web`). This tests the real deployment unit — Vite dev proxies
and `os.DirFS` fallbacks are explicitly *not* exercised.

- `playwright.config.ts`: `baseURL: http://localhost:8090`,
  `webServer: { command: '../scripts/start-stack.sh', url: 'http://localhost:8090/api/health', reuseExistingServer: !process.env.CI }`,
  trace/screenshot/video on failure, JUnit + HTML reporters.

### Helpers

- `admin-session.ts`: login once via `POST /api/admin/login` with Playwright's `request`
  fixture, save `storageState` — no repeated UI logins per test.
- `mailpit.ts`: REST client for `http://localhost:8027` — `waitForMessage(to, subjectContains)`,
  extract `/booking/{token}` / `/cancel/{token}` links from HTML bodies.
- `fixtures.ts`: date picker helpers (navigate to fixed future dates via URL/query),
  slot-grid selection helpers for `react-big-calendar` widget.

### Test list

| File | Flow | Stripe mode |
|---|---|---|
| `spa-navigation.spec.ts` | `/` renders picker; deep links `/booking/:token`, `/admin/*` route correctly; unknown route redirects to `/` | none |
| `public-booking.spec.ts` | pick date → slot grid reflects closures/lead rules → select hours → form validation (email trim, duration bounds) → reach payment step; **full submit**: enter test card in Stripe Payment Element → confirmation page → assert confirmation email in Mailpit → open `/booking/{token}` from the email | real test keys (`sk_test`/`pk_test`) |
| `booking-cancel.spec.ts` | cancel via `/cancel/:token` link from email → status shows cancelled → slot becomes bookable again → cancellation email arrives | fake |
| `admin.spec.ts` | login (bad password → error; good → dashboard); bookings list shows the seeded booking; PATCH status; create closure → public availability reflects it in the picker; settings editor save/persist; users page create/delete; logout → `/admin` redirects to login | fake |
| `payment.spec.ts` | the payment leg above as its own gated test | real test keys; `test.skip(!process.env.STRIPE_TEST_KEY)` |

**Stripe reality check**: the server-side calls (prepare/create/charge) can be faked
(`STRIPE_FAKE=1`), but the browser-side Stripe.js Payment Element confirm always talks to
real `api.stripe.com` — a fake client_secret cannot be confirmed in the browser. Hence the
split: everything except card entry runs hermetically; the true end-to-end payment test
requires Stripe test keys and skips cleanly when they're absent (no key in CI = one
skipped test, not a red suite).

**Real-Stripe safety guardrails** (apply to `payment.spec.ts` only — every other suite
makes **zero** Stripe network calls):

- **Test-mode keys only, enforced in code.** Before doing anything, the test validates
  the configured key: it skips unless the secret key starts with `sk_test_`, and aborts
  with an explicit error if a `sk_live_` key is ever supplied. A misconfigured secret can
  never reach live mode.
- **Test mode is a simulation.** Customers, SetupIntents, and off-session PaymentIntents
  created with `sk_test_` keys are sandbox objects: no funds move, no fees, no payouts,
  and test data is disposable. The test card `4242 4242 4242 4242` is only valid in test
  mode anyway — it cannot produce a real charge even by mistake.
- **No Stripe calls at boot.** The server requires `STRIPE_SECRET_KEY` to start but makes
  no Stripe network call during startup (`cmd/server/main.go`), so a dummy key is safe in
  every fake-mode run.
- **Secret hygiene.** `STRIPE_TEST_KEY` is a GitHub Actions repo secret exposed only to
  the `e2e` job. Optional hardening: use a Stripe *restricted* API key limited to
  SetupIntent/PaymentIntent write on the test account.

## 6. Suite 3 — Docker smoke test

Validates the thing users actually run: the image from `Dockerfile` +
`docker-compose.prod.yml` (SPA embedded at image build, prod env shape, no Mailpit).

```
tests/smoke/run.sh
  docker compose -f docker-compose.prod.yml -p booklab-smoke up -d --build
  wait for /api/health
  hurl --test --variables-file tests/hurl/vars/smoke.env tests/hurl/suites/00_health.hurl \
       tests/hurl/suites/06_spa_routing.hurl \
       + smoke-only: login → GET /api/admin/bookings 200 (proves prod compose DB wiring)
  docker compose -p booklab-smoke down -v
```

Small, fast, and catches the classic failure modes: SPA not embedded (503s),
migrations failing on a fresh volume, wrong prod env wiring. Reuses the Hurl suites with
different variables rather than duplicating requests.

## 7. Optional Suite 4 — background scheduler behavior

The 30-min tick (`scheduler.Start`, `cmd/server/main.go:49`) auto-completes expired
bookings, sends reminders, and auto-charges. A functional version would:

1. Seed a booking whose end time is in the past (via `psql` against the test DB).
2. Make the tick interval configurable (`SCHEDULER_TICK`, default `30m` — one-line
   `config.go` change) and run the app with `SCHEDULER_TICK=2s`.
3. Assert via API + Mailpit: status `confirmed → completed → charged`, reminder +
   receipt emails arrive, exactly one charge (idempotency under the real scheduler).

The concurrency property is already covered by `internal/scheduler/scheduler_test.go`;
this suite would verify the real timed loop against a real binary. Do it last.

## 8. CI wiring (`.github/workflows/ci.yml`)

New jobs alongside the existing `go` and `web` jobs:

| Job | Services | Steps (summary) |
|---|---|---|
| `functional-api` | `postgres:16-alpine`, `axllent/mailpit` | setup-go → `go build` → install hurl (or Hurl image) → `make test-func-api` with host-networked service endpoints → upload JUnit |
| `e2e` | `postgres:16-alpine`, `axllent/mailpit` | pnpm/Node (pin to `.tool-versions`: Node 24, pnpm 11) → install `@playwright/test` + `npx playwright install --with-deps chromium` (cache browsers) → `make build` → `make test-e2e` |
| `docker-smoke` | — (uses Docker on the runner) | `docker compose -f docker-compose.prod.yml up -d --build` → hurl smoke → down |

Notes:

- Same `concurrency` cancel-in-progress policy as today.
- All three jobs are independent → run in parallel with the existing two.
- `STRIPE_TEST_KEY` repo secret (optional, **test-mode only**) enables the payment E2E
  test; without it that single test skips. The `sk_test_`/`sk_live_` guard lives in the
  test itself (see §5), so a bad secret value can never do damage.
- Artifacts: JUnit reports + Playwright traces/screenshots on failure.

GHA specifics:

- **Hurl install**: download a pinned release tarball from Hurl's GitHub releases (no
  third-party actions); alternatively run the suites inside the official
  `ghcr.io/orange-opensource/hurl` image. Locally, Hurl comes from asdf via
  `.tool-versions` like the other tools.
- **Service containers** (`postgres:16-alpine`, `axllent/mailpit`) need explicit `ports:`
  mappings so the runner reaches them at `localhost`. Suites read host/ports from env /
  variables files, so CI mappings don't have to match the local defaults (`5433`/`1026`).
- **Playwright browsers**: cache `~/.cache/ms-playwright` keyed on the pinned Playwright
  version; `npx playwright install --with-deps chromium` installs the system deps.
- **Go/pnpm caching**: reuse the existing setup-go/setup-pnpm cache behavior from the
  current jobs; the functional jobs build the binary first (`go build ./cmd/server`), so
  build caching applies.

## 9. Milestones

| # | Deliverable | Status |
|---|---|---|
| M0 | Shared infra: `tests/compose/docker-compose.test.yml`, scripts, Makefile targets, `GET /api/health` | ✅ done |
| M1 | Hurl suites 00/01/06 (no Stripe needed) + runner | ✅ done |
| M2 | `STRIPE_FAKE` mode + Hurl 02/03/04/05/07 (full lifecycle + email) | ✅ done |
| M3 | Playwright package + config + helpers + `spa-navigation` + `admin` suites | ✅ done |
| M4 | `booking-cancel` + `public-booking` (hermetic parts) | ✅ done |
| M5 | `payment.spec.ts` with real test keys (env-gated, skips cleanly) | ✅ done (skipped without keys) |
| M6 | Docker smoke suite + CI wiring for all jobs | ✅ done |
| M7 (opt) | Scheduler behavior suite (`SCHEDULER_TICK`) | not started |

> **Implementation notes**
> - Running the suites locally surfaced and fixed a real bug: with the default
>   `America/Denver` timezone, `ListClosuresInRange` compared DATE columns
>   against UTC-shifted timestamptz bounds, so same-day closures were missed by
>   availability and booking validation. Fixed via `::date` casts
>   (`internal/db/queries/closures.sql` + `queries.go`), exact per-day bounds in
>   `handleGetAvailability`, and UTC-normalized date interpretation for scanned
>   DATE values (`internal/api/closures.go`), with two regression tests.
> - The test stack is volume-less and disposable (fresh DB per run); the local
>   smoke script adds a SELinux-only Compose override for rootless podman
>   (no-op under Docker/GHA).
> - `payment.spec.ts` additionally requires the publishable key at *build* time
>   (`VITE_STRIPE_PUBLISHABLE_KEY`); `run-tests.sh e2e` rebuilds when the key is
>   present and runs the app against `STRIPE_TEST_SECRET_KEY` with
>   `STRIPE_FAKE=0`.

## 10. Risks & gotchas

- **Login rate limiting** (10 fails / 15 min per IP, and per username — `admin.go:24-25`):
  every request comes from the same IP in tests. `reset-db.sh` truncates `login_attempts`
  between files; the rate-limit test runs last in its file.
- **Scheduler interference**: on a long-lived DB the tick can mutate booking states
  mid-test. Mitigation: own Postgres instance per run + fresh volume (`down -v`).
- **Cookie `Secure` flag**: fine on `http://localhost` in Chromium/Firefox; do not test
  against `127.0.0.1` or container IPs.
- **CORS**: tests drive the app directly on `:8090` (same-origin, no CORS in play). The
  `CORS_ALLOWED_ORIGINS` behavior is already unit-tested (`admin_test.go`); if we ever
  test Vite-dev-origin CORS, it belongs in a dedicated case.
- **Availability/timezone logic**: set the settings row's timezone explicitly in fixtures
  (UTC) — the existing tests surfaced UTC-boundary edge cases here.
- **Hurl cookie capture syntax** varies slightly across versions — pin Hurl in
  `.tool-versions` and verify captures at M1.
- **Accidental live Stripe key**: impossible by construction — `payment.spec.ts` aborts
  unless the secret starts with `sk_test_` (§5), and no other suite touches Stripe at all.
