package api

import (
	"log/slog"
	"net/http"
	"path"
	"strings"
)

// probePathPrefixes are URL path prefixes commonly used by vulnerability scanners.
var probePathPrefixes = []string{
	"/.env",
	"/.git",
	"/.aws",
	"/.svn",
	"/.hg",
	"/wp-",
	"/wordpress",
	"/_profiler",
	"/actuator",
	"/cgi-bin",
	"/vendor/phpunit",
	"/vendor/laravel",
	"/phpmyadmin",
	"/pma",
	"/mysql",
	"/telescope",
	"/debug/",
	"/server-status",
	"/server-info",
	"/autodiscover",
	"/.well-known/security.txt",
}

// probePathSuffixes are file extensions scanners probe for; we have no such assets.
var probePathSuffixes = []string{
	".php",
	".asp",
	".aspx",
	".jsp",
	".cgi",
	".bak",
	".sql",
	".tar",
	".gz",
	".old",
	".swp",
	".config",
}

func isProbePath(rawPath string) bool {
	p := strings.ToLower(path.Clean(rawPath))
	if p == "." {
		p = "/"
	}
	for _, prefix := range probePathPrefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	for _, suffix := range probePathSuffixes {
		if strings.HasSuffix(p, suffix) {
			return true
		}
	}
	return false
}

// isSPARoute reports paths handled by the React client router (see web/src/App.tsx).
func isSPARoute(rawPath string) bool {
	p := path.Clean(rawPath)
	if p == "." {
		p = "/"
	}
	switch {
	case p == "/":
		return true
	case strings.HasPrefix(p, "/booking/"):
		return true
	case strings.HasPrefix(p, "/cancel/"):
		return true
	case p == "/terms":
		return true
	case p == "/privacy":
		return true
	case p == "/admin" || strings.HasPrefix(p, "/admin/"):
		return true
	default:
		return false
	}
}

func isStaticAssetPath(rawPath string) bool {
	p := path.Clean(rawPath)
	if p == "." {
		p = "/"
	}
	return strings.HasPrefix(p, "/assets/")
}

func (s *Server) httpRequestLogLevel(path string, status int) slog.Level {
	if isProbePath(path) {
		return slog.LevelDebug
	}
	if isStaticAssetPath(path) && status < 400 {
		return slog.LevelDebug
	}
	if status == http.StatusNotFound && !isSPARoute(path) {
		return slog.LevelDebug
	}
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}
