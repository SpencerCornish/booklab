# Production deployment

Use **[docker-compose.prod.yml](../docker-compose.prod.yml)** - Postgres plus the BookLab image, no Mailpit. SMTP and Stripe come from your `.env` file.

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

   - `POSTGRES_PASSWORD` - database password (must stay stable; changing it later does not rewrite files already in the Postgres data directory).
   - Optional: `POSTGRES_DATA_PATH` - host path bind-mounted for Postgres (default `./data/postgres` next to `docker-compose.prod.yml`).
   - `VITE_STRIPE_PUBLISHABLE_KEY` - Stripe **publishable** key; baked into the UI at **image build** time.
   - `STRIPE_SECRET_KEY` - Stripe secret key.
   - `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `SMTP_FROM` - real SMTP provider.
   - `APP_URL` - public URL with scheme, e.g. `https://book.example.com` (email links and default CORS).
   - `ADMIN_USER` / `ADMIN_PASS` - first-run bootstrap only; creates that admin if the username does not exist yet. The password must be at least 8 characters or the server refuses to start. Remove from `.env` after the first deploy if you prefer not to keep them on disk.

   Optional: `STRIPE_WEBHOOK_SECRET` (reserved for future use), `CORS_ALLOWED_ORIGINS`, `APP_PUBLISH_PORT` (host port mapped to the app; default `8080`), `TRUSTED_PROXIES` (see step 5).

   You can ignore `DATABASE_URL` in `.env` for this compose file: **`docker-compose.prod.yml` overrides it** so the app talks to the `db` service on the Docker network. Keep `DATABASE_URL` correct for local `go run` / Makefile targets.

4. **Start** the stack (builds the app image using `VITE_STRIPE_PUBLISHABLE_KEY` from `.env`):

   ```bash
   docker compose -f docker-compose.prod.yml up -d --build
   ```

5. **TLS** - the app serves HTTP on the published port (default `8080`). Put a reverse proxy or load balancer in front (HTTPS → `http://127.0.0.1:8080` or your chosen `APP_PUBLISH_PORT`).

   Example (Caddy):

   ```caddy
   book.example.com {
     reverse_proxy 127.0.0.1:8080
   }
   ```

   The app works out each visitor's IP for login and booking rate limits. It only trusts `X-Forwarded-For` from `TRUSTED_PROXIES`, which defaults to loopback and private ranges, so a proxy on the same host (as above) or on the Docker network works without changes. If your proxy or load balancer reaches the app from a public address, set `TRUSTED_PROXIES` to that address; otherwise every visitor shares the proxy's IP and its rate limits. Make sure the proxy appends to or replaces `X-Forwarded-For` (Caddy and nginx's `proxy_add_x_forwarded_for` both do).

   With `APP_URL` on `https://`, the app sends `Strict-Transport-Security`, so browsers will insist on HTTPS for that host for a year.

6. **Open** `APP_URL` in a browser (through your proxy). Admin UI: `/admin`.

## Later: upgrades and backups

- **Upgrade:** `git pull`, then run step 4 again (`up -d --build`). Watch container logs for migration errors on startup.
- **Backup database:** e.g. `docker compose -f docker-compose.prod.yml exec -T db pg_dump -U booklab booklab > backup.sql`
- **Logs:** `docker compose -f docker-compose.prod.yml logs -f app`

## Local / dev compose

For Mailpit and published Postgres on the host, use the default [docker-compose.yml](../docker-compose.yml) and [README.md](../README.md#quick-start).
