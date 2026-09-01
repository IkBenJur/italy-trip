package middleware

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS echoes back the request Origin only when it appears in allowedOrigins.
// A wildcard would let any page on the internet drive the API with a token it
// managed to read, so the allowlist is configuration, not a default.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if trimmed := strings.TrimSpace(origin); trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		c.Header("Vary", "Origin")

		if _, ok := allowed[origin]; ok {
			c.Header("Access-Control-Allow-Origin", origin)
			// Needed so the browser sends and stores the refresh-token cookie on
			// cross-origin requests. Safe only because the origin above is echoed
			// from an allowlist, never a wildcard.
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Header("Access-Control-Max-Age", "600")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// ParseOrigins splits a comma-separated CORS_ORIGIN value and normalises each
// entry into the exact form a browser sends.
func ParseOrigins(value string) []string {
	parts := strings.Split(value, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		if normalized := NormalizeOrigin(part); normalized != "" {
			origins = append(origins, normalized)
		}
	}
	return origins
}

// NormalizeOrigin turns a configured value into the exact string a browser puts
// in the Origin header: scheme, host, optional port — no path, no trailing
// slash.
//
// This matters because Railway's domain variables are bare hostnames. Setting
// CORS_ORIGIN to ${{Frontend.RAILWAY_PUBLIC_DOMAIN}} yields something like
// "app-production.up.railway.app", while the browser sends
// "https://app-production.up.railway.app". Compared literally those never match,
// and the failure is invisible from the server side: the response is a normal
// 200 that the browser then refuses to hand to the page.
func NormalizeOrigin(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	if !strings.Contains(trimmed, "://") {
		trimmed = schemeFor(trimmed) + "://" + trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return ""
	}

	return parsed.Scheme + "://" + parsed.Host
}

// schemeFor guesses the scheme for a bare host. Everything is HTTPS except local
// development — and the camera needs a secure context anyway, so a public origin
// on plain HTTP would not be able to run this app at all.
func schemeFor(hostPort string) string {
	host := hostPort
	if parsedHost, _, err := net.SplitHostPort(hostPort); err == nil {
		host = parsedHost
	}

	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1", "[::1]":
		return "http"
	}
	return "https"
}
