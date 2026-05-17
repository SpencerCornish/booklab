package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
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

	cfg := config.Load()

	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	queries := db.New(pool)
	stripeService := stripesvc.New(cfg.StripeSecretKey)
	emailService := emailsvc.New(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom)

	if user := os.Getenv("ADMIN_USER"); user != "" {
		if pass := os.Getenv("ADMIN_PASS"); pass != "" {
			bootstrapAdmin(ctx, queries, user, pass)
		}
	}

	sched := scheduler.New(queries, emailService, cfg.AppURL)
	go sched.Start(ctx)

	// webFS: prefer embedded FS (populated at build time), fall back to disk
	var webFS fs.FS = webembed.FS
	if webFS == nil {
		const distDir = "internal/webembed/dist"
		if _, err := os.Stat(distDir + "/index.html"); err == nil {
			webFS = os.DirFS(distDir)
		} else {
			log.Printf("warning: %s/index.html not found — SPA will not be served", distDir)
		}
	}

	srv := api.New(cfg, queries, stripeService, emailService, webFS)
	addr := srv.Addr()
	log.Printf("BookLab listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func bootstrapAdmin(ctx context.Context, queries *db.Queries, username, password string) {
	if _, err := queries.GetAdminByUsername(ctx, username); err == nil {
		return // already exists
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("bcrypt: %v", err)
	}
	if _, err := queries.CreateAdminUser(ctx, username, string(hash)); err != nil {
		log.Printf("bootstrap admin: %v", err)
	} else {
		fmt.Printf("Created admin user: %s\n", username)
	}
}
