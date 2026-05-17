package config

import (
	"log"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`
	Port        int    `envconfig:"PORT" default:"8080"`

	StripeSecretKey     string `envconfig:"STRIPE_SECRET_KEY" required:"true"`
	StripeWebhookSecret string `envconfig:"STRIPE_WEBHOOK_SECRET"`

	SMTPHost string `envconfig:"SMTP_HOST" required:"true"`
	SMTPPort int    `envconfig:"SMTP_PORT" default:"587"`
	SMTPUser string `envconfig:"SMTP_USER" required:"true"`
	SMTPPass string `envconfig:"SMTP_PASS" required:"true"`
	SMTPFrom string `envconfig:"SMTP_FROM" required:"true"`

	AppURL string `envconfig:"APP_URL" default:"http://localhost:8080"`

	// CORSAllowedOrigins is a comma-separated list of allowed browser origins (scheme+host+port).
	// If empty, defaults to AppURL only. Required when AllowCredentials is true (cannot use "*").
	CORSAllowedOrigins string `envconfig:"CORS_ALLOWED_ORIGINS" default:""`
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

// AllowedCORSOrigins returns origins for the CORS middleware (trimmed, non-empty entries).
// An empty config value yields a single-element slice containing AppURL.
func (c *Config) AllowedCORSOrigins() []string {
	raw := strings.TrimSpace(c.CORSAllowedOrigins)
	if raw == "" {
		return []string{strings.TrimSpace(c.AppURL)}
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return []string{strings.TrimSpace(c.AppURL)}
	}
	return out
}
