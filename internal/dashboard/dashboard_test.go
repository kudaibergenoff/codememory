package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kudaibergenoff/codememory/internal/analytics"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

func server(t *testing.T) http.Handler {
	t.Helper()
	s, err := sqlite.Open(context.Background(), t.TempDir(), "p")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := analytics.Record(context.Background(), s.DB, analytics.Event{Channel: "mcp", Tool: "memory_search", Agent: analytics.AgentClaude, ResponseBytes: 90}); err != nil {
		t.Fatal(err)
	}
	return Handler(s, "shop")
}

func get(h http.Handler, method, path, host string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPageAndAssetsAreServedWithStrictHeaders(t *testing.T) {
	h := server(t)
	for path, ctype := range map[string]string{"/": "text/html", "/app.js": "text/javascript", "/app.css": "text/css"} {
		rec := get(h, http.MethodGet, path, "127.0.0.1:7777")
		if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), ctype) {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Header().Get("Content-Type"))
		}
		if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "unsafe-inline") {
			t.Fatalf("%s: weak CSP %q", path, csp)
		}
	}
	if rec := get(h, http.MethodGet, "/missing", "localhost:7777"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown path: %d", rec.Code)
	}
}

func TestSummaryIsHonestAboutEstimates(t *testing.T) {
	rec := get(server(t), http.MethodGet, "/api/summary", "[::1]:7777")

	var got struct {
		Note      string          `json:"note"`
		Benchmark json.RawMessage `json:"benchmark"`
		Stats     struct {
			Measured        struct{ Requests int } `json:"measured"`
			VerifiedSavings *int64                 `json:"verified_token_savings"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	if got.Stats.Measured.Requests != 1 || got.Stats.VerifiedSavings != nil || string(got.Benchmark) != "null" || !strings.Contains(got.Note, "no demo data") {
		t.Fatalf("summary %+v", got)
	}
}

func TestRejectsForeignHostsAndWrites(t *testing.T) {
	h := server(t)
	if rec := get(h, http.MethodGet, "/api/summary", "evil.example:7777"); rec.Code != http.StatusForbidden {
		t.Fatalf("DNS-rebinding host allowed: %d", rec.Code)
	}
	if rec := get(h, http.MethodPost, "/api/summary", "127.0.0.1:7777"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST allowed: %d", rec.Code)
	}
}

func TestCheckAddrAllowsLoopbackOnly(t *testing.T) {
	for _, ok := range []string{"127.0.0.1:0", "localhost:8080", "[::1]:0"} {
		if err := CheckAddr(ok); err != nil {
			t.Errorf("%s rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"0.0.0.0:80", ":8080", "example.com:80", "192.168.1.5:80", "nonsense"} {
		if err := CheckAddr(bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

func TestBracketedIPv6HostWithoutPortIsLoopback(t *testing.T) {
	if rec := get(server(t), http.MethodGet, "/", "[::1]"); rec.Code != http.StatusOK {
		t.Fatalf("[::1] rejected: %d", rec.Code)
	}
}
