package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func TestNormalizeOrigin(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "a Railway domain variable, which carries no scheme",
			in:   "italy-frontend-production.up.railway.app",
			want: "https://italy-frontend-production.up.railway.app",
		},
		{
			name: "the same value with the scheme already on it",
			in:   "https://italy-frontend-production.up.railway.app",
			want: "https://italy-frontend-production.up.railway.app",
		},
		{name: "a trailing slash is dropped", in: "https://example.com/", want: "https://example.com"},
		{name: "a path is dropped", in: "https://example.com/app/index.html", want: "https://example.com"},
		{name: "surrounding whitespace", in: "  https://example.com \n", want: "https://example.com"},
		{name: "a port is kept", in: "https://example.com:8443", want: "https://example.com:8443"},

		// Local development is the one place that is not HTTPS.
		{name: "bare localhost with a port", in: "localhost:5173", want: "http://localhost:5173"},
		{name: "localhost with an explicit scheme", in: "http://localhost:5173", want: "http://localhost:5173"},
		{name: "bare loopback IP", in: "127.0.0.1:5173", want: "http://127.0.0.1:5173"},
		{name: "an explicit https localhost is respected", in: "https://localhost:5173", want: "https://localhost:5173"},

		{name: "empty", in: "", want: ""},
		{name: "whitespace only", in: "   ", want: ""},
		{name: "a lone slash is not a host", in: "/", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeOrigin(tt.in); got != tt.want {
				t.Fatalf("NormalizeOrigin(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseOrigins(t *testing.T) {
	got := ParseOrigins("localhost:5173, https://italy.up.railway.app , ,italy-api.up.railway.app")
	want := []string{
		"http://localhost:5173",
		"https://italy.up.railway.app",
		"https://italy-api.up.railway.app",
	}

	if len(got) != len(want) {
		t.Fatalf("ParseOrigins returned %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ParseOrigins[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// newCORSRouter builds a router with only the CORS middleware on it.
func newCORSRouter(configured string) *gin.Engine {
	router := gin.New()
	router.Use(CORS(ParseOrigins(configured)))
	router.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	return router
}

func request(t *testing.T, router *gin.Engine, method, origin string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/health", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if method == http.MethodOptions {
		req.Header.Set("Access-Control-Request-Method", "GET")
	}
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	return res
}

// TestCORSAcceptsARailwayDomainVariable is the regression test for the whole
// point of this file: Railway supplies a bare hostname, the browser sends a full
// origin, and the two must still match.
func TestCORSAcceptsARailwayDomainVariable(t *testing.T) {
	router := newCORSRouter("italy-frontend-production.up.railway.app")
	const browserOrigin = "https://italy-frontend-production.up.railway.app"

	for _, method := range []string{http.MethodGet, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			res := request(t, router, method, browserOrigin)
			if got := res.Header().Get("Access-Control-Allow-Origin"); got != browserOrigin {
				t.Fatalf("Access-Control-Allow-Origin = %q, want %q — the browser would block this response",
					got, browserOrigin)
			}
		})
	}
}

func TestCORSRefusesEveryOtherOrigin(t *testing.T) {
	router := newCORSRouter("https://italy.up.railway.app")

	for _, origin := range []string{
		"https://evil.example.com",
		// A lookalike that merely ends with the allowed host.
		"https://evil-italy.up.railway.app",
		// A subdomain of the allowed host: still a different origin.
		"https://sub.italy.up.railway.app",
		// The right host on the wrong scheme.
		"http://italy.up.railway.app",
		// The right host on a different port.
		"https://italy.up.railway.app:8443",
	} {
		t.Run(origin, func(t *testing.T) {
			res := request(t, router, http.MethodGet, origin)
			if got := res.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Fatalf("Access-Control-Allow-Origin = %q for %s, want no header at all", got, origin)
			}
		})
	}
}

func TestCORSNeverEmitsAWildcard(t *testing.T) {
	// A wildcard would let any page drive the API with a token it had read.
	for _, configured := range []string{"*", "https://italy.up.railway.app"} {
		router := newCORSRouter(configured)
		res := request(t, router, http.MethodGet, "https://anything.example.com")
		if got := res.Header().Get("Access-Control-Allow-Origin"); got == "*" {
			t.Fatalf("configured %q produced a wildcard Allow-Origin", configured)
		}
	}
}

func TestCORSAlwaysVariesOnOrigin(t *testing.T) {
	// Without this a shared cache could serve one origin's response to another.
	router := newCORSRouter("https://italy.up.railway.app")

	for _, origin := range []string{"https://italy.up.railway.app", "https://evil.example.com", ""} {
		res := request(t, router, http.MethodGet, origin)
		if got := res.Header().Get("Vary"); got != "Origin" {
			t.Errorf("Vary = %q for origin %q, want Origin", got, origin)
		}
	}
}

func TestCORSPreflightShortCircuits(t *testing.T) {
	router := newCORSRouter("https://italy.up.railway.app")

	res := request(t, router, http.MethodOptions, "https://italy.up.railway.app")
	if res.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", res.Code)
	}
	if got := res.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("preflight advertises no allowed headers, so Authorization would be rejected")
	}
	if got := res.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("preflight advertises no allowed methods")
	}
}

func TestCORSAllowsMultipleOrigins(t *testing.T) {
	// The real deployment shape: local dev plus the Railway frontend.
	router := newCORSRouter("localhost:5173,italy-frontend-production.up.railway.app")

	for _, origin := range []string{
		"http://localhost:5173",
		"https://italy-frontend-production.up.railway.app",
	} {
		res := request(t, router, http.MethodGet, origin)
		if got := res.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, origin)
		}
	}
}

func TestCORSWithNoOriginHeaderIsUnaffected(t *testing.T) {
	// curl, health checks and the Railway probe send no Origin.
	router := newCORSRouter("https://italy.up.railway.app")

	res := request(t, router, http.MethodGet, "")
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	if got := res.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want no header", got)
	}
}
