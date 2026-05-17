# Contributing to BookLab

Thanks for your interest in contributing.

## Getting started

1. Fork the repository and clone your fork.
2. Copy environment templates and fill in secrets for local development:

   ```bash
   cp .env.example .env
   cp web/.env.example web/.env
   ```

   See [README.md](README.md) for variable descriptions.

3. Install tool versions (recommended: [asdf](https://asdf-vm.com/) using [.tool-versions](.tool-versions)):

   ```bash
   asdf plugin add pnpm  # once per machine
   make install-tools    # or: asdf install
   ```

4. Start Postgres (and optional Mailpit), then run the API and frontend:

   ```bash
   make db                 # Postgres + Mailpit in Docker
   make dev                # Go API + Vite (parallel), or use dev-api / dev-web separately
   ```

   Or follow the Docker-only path in the README.

## What to run before opening a PR

From the repository root:

```bash
make lint    # Go vet or golangci-lint + frontend tsc
make test    # Go tests
```

CI also runs `gofmt`, `go vet`, `go test`, `go build`, and the web typecheck/build — matching that locally reduces review churn.

## Pull requests

- Keep changes focused on one concern when possible.
- Describe the problem and the approach in the PR body; link related issues if any.
- If you change user-visible behavior or configuration, update the README or `.env.example` as needed.

## Code style

- **Go:** `go fmt` / standard library patterns; follow existing package layout under `internal/` and `cmd/`.
- **TypeScript/React:** match existing components and hooks; run Prettier if you use `make fmt`.

## Questions

Open a [discussion](https://github.com/SpencerCornish/booklab/discussions) or an issue for design questions before large refactors.
