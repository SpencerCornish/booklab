# BookLab

A self-hosted hourly resource booking system with post-use Stripe billing. Built for community spaces: darkrooms, makerspaces, studios, etc.

## Features

- **Public booking page** — date picker, visual time-slot grid, multi-hour selection
- **Stripe card-on-file** — card saved at booking, charged after the session by an admin
- **Admin panel** — bookings list, charge flow, closure management, settings editor
- **Email notifications** — confirmation, reminder, cancellation, receipt (SMTP)
- **DB-level conflict prevention** — PostgreSQL exclusion constraint prevents double-bookings
- **Configurable** — all branding via env vars, suitable for any resource

## Quick Start

```bash
cp .env.example .env
# Fill in STRIPE_SECRET_KEY, SMTP_*, ADMIN_USER, ADMIN_PASS
docker compose up --build
```

The app will be available at `http://localhost:8080`.
Admin panel: `http://localhost:8080/admin`

## Configuration

All configuration is via environment variables (see `.env.example`):

| Variable | Description |
|---|---|
| `DATABASE_URL` | PostgreSQL connection string |
| `PORT` | HTTP port (default: 8080) |
| `STRIPE_SECRET_KEY` | Stripe secret key (`sk_...`) |
| `SMTP_HOST` / `SMTP_PORT` / `SMTP_USER` / `SMTP_PASS` / `SMTP_FROM` | Email config |
| `APP_URL` | Public URL (used in email links and as the default CORS origin) |
| `CORS_ALLOWED_ORIGINS` | Optional comma-separated list of allowed browser origins for credentialed API requests; defaults to `APP_URL` when empty |
| `ADMIN_USER` / `ADMIN_PASS` | Bootstrap admin credentials (first run only) |
| `STRIPE_WEBHOOK_SECRET` | Optional; reserved for future Stripe webhook support |
| `POSTGRES_PASSWORD` / `POSTGRES_HOST_PORT` | Used by `docker compose` for the local/dev Postgres service (see `.env.example`) |
| `VITE_STRIPE_PUBLISHABLE_KEY` | Stripe publishable key; passed into the Docker **build** for embedded frontend (see `.env.example`) |
| `APP_PUBLISH_PORT` | Production compose only: host port for the app (default `8080`; see `docker-compose.prod.yml`) |
| `POSTGRES_DATA_PATH` | Production compose only: host directory for Postgres data (default `./data/postgres`) |

## Frontend Environment

Add a `.env` file in `web/` for local development:

```
VITE_STRIPE_PUBLISHABLE_KEY=pk_test_...
```

Docker builds (dev or prod compose) read `VITE_STRIPE_PUBLISHABLE_KEY` from the **root** `.env` via Compose build args—you do not need `web/.env` on the server for production.

## Architecture

```
Browser → Go server (:8080)
              ├── Serves React SPA (embedded via embed.FS)
              ├── /api/* → REST handlers
              │       ├── Public: availability, create/cancel/view booking
              │       └── Admin: bookings, charge, closures, settings
              ├── PostgreSQL (migrations run on startup)
              ├── Stripe API (SetupIntent + PaymentIntent)
              └── SMTP (confirmation, reminder, cancellation, receipt)
```

## Development

Install pinned tools with [asdf](https://asdf-vm.com/): Go, Node.js, and **pnpm** (see `.tool-versions`).

```bash
# One-time: register the pnpm plugin, then install all tools from .tool-versions
asdf plugin add pnpm
make install-tools
```

Then:

```bash
# Start Postgres
docker compose up db -d

# Run backend
go run ./cmd/server

# Run frontend (separate terminal; pnpm comes from asdf)
cd web && pnpm install && pnpm run dev
```

Or run `make setup` once to copy env files and install frontend dependencies (requires `pnpm` on your PATH from asdf).

## Production deployment

Use **[docker-compose.prod.yml](docker-compose.prod.yml)** (Postgres + app, real SMTP from `.env`). Step-by-step: [docs/deployment.md](docs/deployment.md).

```bash
cp .env.example .env   # then edit: Stripe, SMTP, APP_URL, POSTGRES_PASSWORD, VITE_STRIPE_PUBLISHABLE_KEY, …
docker compose -f docker-compose.prod.yml up -d --build
```

Put HTTPS in front of the published port (default host `8080`), for example with Caddy:

```caddy
book.example.com {
  reverse_proxy localhost:8080
}
```

## License

MIT — see [LICENSE.md](LICENSE.md).
