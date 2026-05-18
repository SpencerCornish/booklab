package main

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"

	"github.com/spencercornish/booklab/internal/api"
	"github.com/spencercornish/booklab/internal/config"
	"github.com/spencercornish/booklab/internal/db"
	emailsvc "github.com/spencercornish/booklab/internal/email"
	"github.com/spencercornish/booklab/internal/scheduler"
	stripesvc "github.com/spencercornish/booklab/internal/stripe"
	"github.com/spencercornish/booklab/internal/webembed"
)

func main() {
	_ = godotenv.Load()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg := config.Load()

	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	queries := db.New(pool)
	stripeService := stripesvc.New(cfg.StripeSecretKey, logger)
	emailService := emailsvc.New(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom, logger)

	if user := os.Getenv("ADMIN_USER"); user != "" {
		if pass := os.Getenv("ADMIN_PASS"); pass != "" {
			bootstrapAdmin(ctx, queries, user, pass)
		}
	}

	sched := scheduler.New(queries, emailService, stripeService, cfg.AppURL, logger)
	go sched.Start(ctx)

	// webFS: prefer embedded FS (populated at build time), fall back to disk
	var webFS fs.FS = webembed.FS
	if webFS == nil {
		const distDir = "internal/webembed/dist"
		if _, err := os.Stat(distDir + "/index.html"); err == nil {
			webFS = os.DirFS(distDir)
		} else {
			slog.Warn("spa dist missing", "path", distDir+"/index.html")
		}
	}

	srv := api.New(cfg, queries, stripeService, emailService, webFS, logger)
	addr := srv.Addr()
	slog.Info("booklab listening", "addr", addr)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		slog.Error("http server failed", "error", err)
		os.Exit(1)
	}
}

func bootstrapAdmin(ctx context.Context, queries *db.Queries, username, password string) {
	if _, err := queries.GetAdminByUsername(ctx, username); err == nil {
		return // already exists
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("bootstrap admin bcrypt failed", "error", err)
		os.Exit(1)
	}
	if _, err := queries.CreateAdminUser(ctx, username, string(hash)); err != nil {
		slog.Error("bootstrap admin create failed", "username", username, "error", err)
	} else {
		slog.Info("bootstrap admin created", "username", username)
	}
}
