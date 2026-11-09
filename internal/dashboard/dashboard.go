// Package dashboard serves a read-only local view of measurements over
// loopback HTTP. It loads no external resources and sends no telemetry.
package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/kudaibergenoff/codememory/internal/analytics"
	"github.com/kudaibergenoff/codememory/internal/search"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

//go:embed assets
var assets embed.FS

const (
	note              = "All values are local measurements from this machine; there is no demo data. Estimated tokens are a size proxy of responses; verified savings need client usage data and are N/A."
	csp               = "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
	window            = 30 * 24 * time.Hour
)

type summary struct {
	Project   string           `json:"project"`
	Generated string           `json:"generated_at"`
	Note      string           `json:"note"`
	Stats     analytics.Report `json:"stats"`
	Daily     []analytics.Row  `json:"daily"`
	Footprint sqlite.Footprint `json:"footprint"`
	Benchmark json.RawMessage  `json:"benchmark"`
}

// CheckAddr accepts only loopback listen addresses.
func CheckAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid --addr %q: %w", addr, err)
	}
	if !isLoopback(host) {
		return fmt.Errorf("dashboard listens on loopback only (127.0.0.1, ::1 or localhost), not %q", host)
	}
	return nil
}

func isLoopback(host string) bool {
	host = strings.TrimSuffix(strings.Trim(host, "[]"), ".")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Handler serves the page, its assets and /api/summary for project.
func Handler(s *sqlite.Store, project string) http.Handler {
	mux := http.NewServeMux()
	for path, file := range map[string]string{"/": "assets/index.html", "/app.js": "assets/app.js", "/app.css": "assets/app.css"} {
		mux.HandleFunc("GET "+path, serveAsset(file))
	}
	mux.HandleFunc("GET /api/summary", func(w http.ResponseWriter, r *http.Request) {
		data, err := collect(r.Context(), s, project)
		if err != nil {
			slog.Warn("dashboard summary failed", "error", err)
			http.Error(w, "summary unavailable", http.StatusInternalServerError)
			return
		}
		payload, err := search.Encode(data)
		if err != nil {
			http.Error(w, "summary unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(payload)
	})
	return guard(mux)
}

// guard rejects non-loopback Host headers (DNS rebinding) and sets strict headers.
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if !isLoopback(host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func serveAsset(name string) http.HandlerFunc {
	types := map[string]string{"assets/index.html": "text/html; charset=utf-8", "assets/app.js": "text/javascript; charset=utf-8", "assets/app.css": "text/css; charset=utf-8"}
	return func(w http.ResponseWriter, r *http.Request) {
		if name == "assets/index.html" && r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		b, err := assets.ReadFile(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", types[name])
		_, _ = w.Write(b)
	}
}

func collect(ctx context.Context, s *sqlite.Store, project string) (summary, error) {
	f := analytics.Filter{Since: time.Now().Add(-window)}
	out := summary{Project: project, Generated: time.Now().UTC().Format(time.RFC3339), Note: note, Benchmark: json.RawMessage("null")}
	var err error
	if out.Stats, err = analytics.Stats(ctx, s.DB, f); err != nil {
		return out, err
	}
	if out.Daily, err = analytics.Daily(ctx, s.DB, f); err != nil {
		return out, err
	}
	if out.Footprint, err = s.Footprint(ctx); err != nil {
		return out, err
	}
	raw, ok, err := s.LatestBenchmark(ctx)
	if err != nil {
		return out, err
	}
	if ok {
		out.Benchmark = raw
	}
	return out, nil
}

// Serve listens on a loopback addr until ctx ends; ready receives the URL.
func Serve(ctx context.Context, s *sqlite.Store, project, addr string, ready func(url string)) error {
	if err := CheckAddr(addr); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	srv := &http.Server{Handler: Handler(s, project), ReadHeaderTimeout: readHeaderTimeout, ReadTimeout: readTimeout, WriteTimeout: writeTimeout, IdleTimeout: idleTimeout}
	ready("http://" + ln.Addr().String() + "/")
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
		case <-done: // Serve returned on its own
			return
		}
		shutdown, cancel := context.WithTimeout(context.Background(), readHeaderTimeout)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
