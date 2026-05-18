# BookLab Makefile
# Usage: make <target>
# Run `make help` to see all available targets.

.DEFAULT_GOAL := help
.PHONY: help dev dev-api dev-web db db-stop db-reset \
        build build-web build-api \
        docker-build docker-up docker-down docker-logs \
        migrate seed-admin \
        lint fmt test tidy \
        clean

# ── Colors ──────────────────────────────────────────────────────────────────
CYAN  := \033[0;36m
RESET := \033[0m

# ── Config ───────────────────────────────────────────────────────────────────
GO      := go
PNPM    := pnpm
DC      := docker compose
WEB_DIR := web

# Load DATABASE_URL from .env if present (for migrate/seed targets)
-include .env
export

# ─────────────────────────────────────────────────────────────────────────────

##@ Development

dev: db ## Start everything for local dev (DB + Go API + Vite frontend)
	@echo "$(CYAN)Starting API and frontend in parallel...$(RESET)"
	@$(MAKE) -j2 dev-api dev-web

dev-api: ## Run the Go server (hot-reload via air if installed, else plain go run)
	@if command -v air > /dev/null 2>&1; then \
		echo "$(CYAN)Starting API with air (hot-reload)...$(RESET)"; \
		air; \
	else \
		echo "$(CYAN)Starting API (install 'air' for hot-reload: go install github.com/air-verse/air@latest)...$(RESET)"; \
		$(GO) run ./cmd/server; \
	fi

dev-web: ## Run the Vite dev server (proxies /api to :8080)
	@echo "$(CYAN)Starting Vite dev server...$(RESET)"
	@cd $(WEB_DIR) && $(PNPM) run dev

db: ## Start Postgres + Mailpit in Docker (detached)
	@echo "$(CYAN)Starting Postgres and Mailpit...$(RESET)"
	@$(DC) up db mailpit -d
	@echo "$(CYAN)Waiting for Postgres to be ready...$(RESET)"
	@until $(DC) exec db pg_isready -U booklab -q; do sleep 1; done
	@echo "$(CYAN)Postgres is ready. Mailpit UI → http://localhost:8025$(RESET)"

mailpit: ## Start Mailpit only (email preview at http://localhost:8025)
	@echo "$(CYAN)Starting Mailpit...$(RESET)"
	@$(DC) up mailpit -d
	@echo "$(CYAN)Mailpit UI → http://localhost:8025$(RESET)"

db-stop: ## Stop Postgres + Mailpit
	@echo "$(CYAN)Stopping Postgres and Mailpit...$(RESET)"
	@$(DC) stop db mailpit

db-reset: ## Destroy and recreate the Postgres volume (WARNING: deletes all data)
	@echo "$(CYAN)Resetting database...$(RESET)"
	@$(DC) down -v
	@$(MAKE) db

seed-admin: ## Create the initial admin user (reads ADMIN_USER / ADMIN_PASS from .env)
	@if [ -z "$(ADMIN_USER)" ] || [ -z "$(ADMIN_PASS)" ]; then \
		echo "Set ADMIN_USER and ADMIN_PASS in .env first."; exit 1; \
	fi
	@echo "$(CYAN)Seeding admin user '$(ADMIN_USER)'...$(RESET)"
	@ADMIN_USER=$(ADMIN_USER) ADMIN_PASS=$(ADMIN_PASS) $(GO) run ./cmd/server & \
		sleep 2 && kill %1 2>/dev/null; true

##@ Build

build: build-web build-api ## Build frontend then Go binary

build-web: ## Build the React SPA into internal/webembed/dist
	@echo "$(CYAN)Building frontend...$(RESET)"
	@cd $(WEB_DIR) && $(PNPM) install --frozen-lockfile && $(PNPM) run build

build-api: ## Compile the Go binary (./booklab)
	@echo "$(CYAN)Building Go binary...$(RESET)"
	@$(GO) build -o booklab ./cmd/server

##@ Docker

docker-build: ## Build the Docker image
	@echo "$(CYAN)Building Docker image...$(RESET)"
	@$(DC) build

docker-up: ## Build & start all services via docker-compose
	@echo "$(CYAN)Starting all services...$(RESET)"
	@$(DC) up --build -d

docker-down: ## Stop and remove containers (keeps volumes)
	@echo "$(CYAN)Stopping containers...$(RESET)"
	@$(DC) down

docker-logs: ## Tail logs from all docker-compose services
	@$(DC) logs -f

##@ Code Quality

tidy: ## Run go mod tidy
	@echo "$(CYAN)Tidying Go modules...$(RESET)"
	@$(GO) mod tidy

fmt: ## Format Go and frontend code
	@echo "$(CYAN)Formatting Go...$(RESET)"
	@$(GO) fmt ./...
	@echo "$(CYAN)Formatting frontend...$(RESET)"
	@cd $(WEB_DIR) && $(PNPM) exec prettier --write "src/**/*.{ts,tsx}" 2>/dev/null || true

lint: ## Lint Go (golangci-lint) and frontend (tsc type-check)
	@echo "$(CYAN)Linting Go...$(RESET)"
	@if command -v golangci-lint > /dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed; running go vet instead."; \
		$(GO) vet ./...; \
	fi
	@echo "$(CYAN)Type-checking frontend...$(RESET)"
	@cd $(WEB_DIR) && $(PNPM) exec tsc --noEmit

test: ## Run Go tests
	@echo "$(CYAN)Running tests...$(RESET)"
	@$(GO) test ./... -v

##@ Misc

clean: ## Remove built artifacts
	@echo "$(CYAN)Cleaning...$(RESET)"
	@rm -f booklab
	@rm -rf internal/webembed/dist/*
	@echo 'Run make build-web to populate this directory.' > internal/webembed/dist/placeholder.txt

install-tools: ## Install Go, Node, pnpm, etc. from .tool-versions via asdf (add plugins first)
	@echo "$(CYAN)Installing tools via asdf...$(RESET)"
	@asdf install

setup: ## First-time setup: copy .env.example, install frontend deps
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "$(CYAN)Created .env from .env.example - fill in your secrets.$(RESET)"; \
	else \
		echo ".env already exists."; \
	fi
	@if [ ! -f $(WEB_DIR)/.env ]; then \
		cp $(WEB_DIR)/.env.example $(WEB_DIR)/.env; \
		echo "$(CYAN)Created web/.env - add your Stripe publishable key.$(RESET)"; \
	fi
	@echo "$(CYAN)Installing frontend dependencies...$(RESET)"
	@command -v $(PNPM) >/dev/null 2>&1 || { \
		echo "$(CYAN)pnpm not found.$(RESET) Add the asdf plugin and install tools:"; \
		echo "  asdf plugin add pnpm"; \
		echo "  make install-tools"; \
		exit 1; \
	}
	@cd $(WEB_DIR) && $(PNPM) install

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make $(CYAN)<target>$(RESET)\n"} \
		/^[a-zA-Z_-]+:.*?##/ { printf "  $(CYAN)%-18s$(RESET) %s\n", $$1, $$2 } \
		/^##@/ { printf "\n$(CYAN)%s$(RESET)\n", substr($$0, 5) }' $(MAKEFILE_LIST)
