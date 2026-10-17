package config

import (
	"fmt"
	"log"
	"net/netip"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`
	Port        int    `envconfig:"PORT" default:"8080"`

	StripeSecretKey string `envconfig:"STRIPE_SECRET_KEY" required:"true"`

	SMTPHost string `envconfig:"SMTP_HOST" required:"true"`
	SMTPPort int    `envconfig:"SMTP_PORT" default:"587"`
	SMTPUser string `envconfig:"SMTP_USER" required:"true"`
	SMTPPass string `envconfig:"SMTP_PASS" required:"true"`
	SMTPFrom string `envconfig:"SMTP_FROM" required:"true"`

	AppURL string `envconfig:"APP_URL" default:"http://localhost:8080"`

	// CORSAllowedOrigins is a comma-separated list of allowed browser origins (scheme+host+port).
	// If empty, defaults to AppURL only. Required when AllowCredentials is true (cannot use "*").
	CORSAllowedOrigins string `envconfig:"CORS_ALLOWED_ORIGINS" default:""`

	// TrustedProxies is a comma-separated list of IPs or CIDR ranges whose X-Forwarded-For
	// header is believed when working out the client IP. Defaults to loopback and private
	// ranges, which covers a reverse proxy on the same host or Docker network. Empty trusts none.
	TrustedProxies string `envconfig:"TRUSTED_PROXIES" default:"127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fc00::/7"`
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
	if _, err := cfg.TrustedProxyPrefixes(); err != nil {
		log.Fatalf("config: %v", err)
	}
	return cfg
}

// SessionCookieName is the name of the admin session cookie.
const SessionCookieName = "booklab_session"

// SessionDuration is how long an admin session lasts.
const SessionDuration = 24 * time.Hour

// MinAdminPasswordLength is the shortest password accepted for an admin account.
const MinAdminPasswordLength = 8

// TrustedProxyPrefixes parses TrustedProxies. Bare IPs are treated as single-address ranges.
func (c *Config) TrustedProxyPrefixes() ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, part := range strings.Split(c.TrustedProxies, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(part); err == nil {
			out = append(out, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(part)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXIES: invalid IP or CIDR %q", part)
		}
		addr = addr.Unmap()
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}

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
