# Functional tests

Out-of-process test suites that exercise the **real binary**, **real database**,
**embedded SPA**, and **real email delivery** (Mailpit). They complement the
in-process Go tests in `internal/` — see
[docs/functional-testing-plan.md](../docs/functional-testing-plan.md) for the
design rationale.

| Suite | Command | What it covers |
|---|---|---|
| Hurl API | `make test-func-api` | ~150 black-box HTTP requests against the running binary: auth/CSRF/sessions, rate limiting, availability, booking lifecycle, admin charges, settings/users/closures, SPA routing, email content |
| Playwright E2E | `make test-e2e` | Cycles the UI: booking form, cancel flow, admin panel (bookings/closures/settings/users), navigation. The real-Stripe payment spec (`payment.spec.ts`) runs only with test-mode keys |
| Docker smoke | `make test-smoke` | Builds `docker-compose.prod.yml` (the actual deployment unit) and probes health + SPA + admin wiring |

## Running locally

Requirements: Go, pnpm (asdf via `.tool-versions`), Docker, Hurl 8.0.1
(`asdf plugin add hurl && asdf install`).

```bash
make test-func-api   # stack up → build → run app → Hurl suites → teardown
make test-e2e        # stack up → build → run app → Playwright → teardown
make test-smoke      # production compose build + probe
```

Each command tears everything down afterwards, even on failure. `make stack-up`
/ `stack-down` / `stack-nuke` manage the test stack manually (it never touches
development ports or data: Postgres `:5433`, Mailpit `:1026`/UI `:8027`, app
`:8090`).

## How state stays deterministic

- The test stack is disposable: every run starts from a fresh database and
  Mailpit.
- Between Hurl files, `tests/scripts/reset-db.sh` truncates bookings, closures,
  sessions, and login attempts, and restores the settings row to defaults —
  suite order never matters.
- All booking dates are computed to be safely in the future inside the default
  bookable window (09:00–21:00 America/Denver).

## Stripe handling

- **Default (hermetic):** `STRIPE_FAKE=1` swaps the Stripe client for a fake
  (`internal/stripe/fake.go`) — zero network calls. All booking, charge, and
  email flows run without keys.
- **Payment spec:** `tests/e2e/tests/payment.spec.ts` drives the real
  Stripe.js Payment flow with a test card. It requires *test-mode* keys:

  ```bash
  STRIPE_TEST_SECRET_KEY=sk_test_…      # server-side
  STRIPE_TEST_PUBLISHABLE_KEY=pk_test_… # baked into the SPA build
  make test-e2e
  ```

  Without them the spec skips. The spec itself aborts on any non-`sk_test_`
  key — it cannot touch live Stripe.

## Layout

```
tests/
  compose/docker-compose.test.yml   isolated Postgres + Mailpit
  scripts/                          stack orchestration (lib.sh, run-tests.sh, …)
  hurl/vars/                        variables for the Hurl suites
  hurl/suites/                      00…07 Hurl files (see header comments)
  e2e/                              Playwright package (config, helpers, specs)
  smoke/                            production-compose smoke runner
```