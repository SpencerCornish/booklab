# Production deployment

Use **[docker-compose.prod.yml](../docker-compose.prod.yml)** — Postgres plus the BookLab image, no Mailpit. SMTP and Stripe come from your `.env` file.

## Steps

1. **Install** Docker and Docker Compose v2 on the server.

2. **Clone** this repository and enter the directory.

   ```bash
   git clone <your-repo-url> booklab
   cd booklab
   ```

3. **Create** `.env` from the example and fill in every value your environment needs.

   ```bash
   cp .env.example .env
   ```

   Required for production (see comments in [.env.example](../.env.example)):

   - `POSTGRES_PASSWORD` — database password (must stay stable; changing it later does not rewrite files already in the Postgres data directory).
   - Optional: `POSTGRES_DATA_PATH` — host path bind-mounted for Postgres (default `./data/postgres` next to `docker-compose.prod.yml`).
   - `VITE_STRIPE_PUBLISHABLE_KEY` — Stripe **publishable** key; baked into the UI at **image build** time.
   - `STRIPE_SECRET_KEY` — Stripe secret key.
   - `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `SMTP_FROM` — real SMTP provider.
   - `APP_URL` — public URL with scheme, e.g. `https://book.example.com` (email links and default CORS).
   - `ADMIN_USER` / `ADMIN_PASS` — first-run bootstrap only; creates that admin if the username does not exist yet. Remove from `.env` after the first deploy if you prefer not to keep them on disk.

   Optional: `STRIPE_WEBHOOK_SECRET` (reserved for future use), `CORS_ALLOWED_ORIGINS`, `APP_PUBLISH_PORT` (host port mapped to the app; default `8080`).

   You can ignore `DATABASE_URL` in `.env` for this compose file: **`docker-compose.prod.yml` overrides it** so the app talks to the `db` service on the Docker network. Keep `DATABASE_URL` correct for local `go run` / Makefile targets.

4. **Start** the stack (builds the app image using `VITE_STRIPE_PUBLISHABLE_KEY` from `.env`):

   ```bash
   docker compose -f docker-compose.prod.yml up -d --build
   ```

5. **TLS** — the app serves HTTP on the published port (default `8080`). Put a reverse proxy or load balancer in front (HTTPS → `http://127.0.0.1:8080` or your chosen `APP_PUBLISH_PORT`).

   Example (Caddy):

   ```caddy
   book.example.com {
     reverse_proxy 127.0.0.1:8080
   }
   ```

6. **Open** `APP_URL` in a browser (through your proxy). Admin UI: `/admin`.

## Later: upgrades and backups

- **Upgrade:** `git pull`, then run step 4 again (`up -d --build`). Watch container logs for migration errors on startup.
- **Backup database:** e.g. `docker compose -f docker-compose.prod.yml exec -T db pg_dump -U booklab booklab > backup.sql`
- **Logs:** `docker compose -f docker-compose.prod.yml logs -f app`

## Local / dev compose

For Mailpit and published Postgres on the host, use the default [docker-compose.yml](../docker-compose.yml) and [README.md](../README.md#quick-start).
