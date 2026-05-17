package config

import (
	"log"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	DatabaseURL   string `envconfig:"DATABASE_URL" required:"true"`
	Port          int    `envconfig:"PORT" default:"8080"`
	SessionSecret string `envconfig:"SESSION_SECRET" required:"true"`

	StripeSecretKey      string `envconfig:"STRIPE_SECRET_KEY" required:"true"`
	StripeWebhookSecret  string `envconfig:"STRIPE_WEBHOOK_SECRET"`

	SMTPHost string `envconfig:"SMTP_HOST" required:"true"`
	SMTPPort int    `envconfig:"SMTP_PORT" default:"587"`
	SMTPUser string `envconfig:"SMTP_USER" required:"true"`
	SMTPPass string `envconfig:"SMTP_PASS" required:"true"`
	SMTPFrom string `envconfig:"SMTP_FROM" required:"true"`

	AppURL string `envconfig:"APP_URL" default:"http://localhost:8080"`
}

var cfg *Config

func Load() *Config {
	if cfg != nil {
		return cfg
	}
	cfg = &Config{}
	if err := envconfig.Process("", cfg); err != nil {
		log.Fatalf("config: %v", err)
	}
	return cfg
}

// SessionCookieName is the name of the admin session cookie.
const SessionCookieName = "booklab_session"

// SessionDuration is how long an admin session lasts.
const SessionDuration = 24 * time.Hour
