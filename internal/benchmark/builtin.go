package benchmark

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/memory"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

//go:embed testdata/shop
var fixture embed.FS

const fixtureRoot = "testdata/shop"

// builtinSuite covers positive retrieval plus negative cases: a task with no
// relevant code and a memory whose cited file changed after it was saved.
var builtinSuite = Suite{Tasks: []Task{
	{ID: "refund-limit", Task: "add a partial refund limit to RefundPayment", ExpectFiles: []string{"payments/refund.go"}},
	{ID: "invoice-discount", Task: "apply a percentage discount when creating an invoice", ExpectFiles: []string{"invoices/invoice.py"}},
	{ID: "session-expiry", Task: "expire user sessions after inactivity", ExpectFiles: []string{"auth/session.ts"}},
	{ID: "receipt-email", Task: "send the receipt email after a successful charge", ExpectFiles: []string{"notifications/email.go", "payments/charge.go"}},
	{ID: "release-stock", Task: "release reserved stock when an order is cancelled", ExpectFiles: []string{"inventory/stock.py"}},
	{ID: "no-relevant-code", Task: "configure the kubernetes helm chart", ExpectNoMatch: true},
	{ID: "stale-memory", Task: "refund limit for a captured charge", ExpectMemoryStatus: memory.StatusStale},
}}

// Builtin runs the embedded suite in a temporary copy of the fixture project.
// The user's project is not touched.
func Builtin(ctx context.Context) (Report, error) {
	dir, err := os.MkdirTemp("", "codememory-benchmark-")
	if err != nil {
		return Report{}, err
	}
	defer os.RemoveAll(dir)
	if err = copyFixture(dir); err != nil {
		return Report{}, err
	}
	root, err := config.Root(dir)
	if err != nil {
		return Report{}, err
	}
	if err = config.Init(root); err != nil {
		return Report{}, err
	}
	s, err := sqlite.Open(ctx, filepath.Join(root, ".codememory"), root)
	if err != nil {
		return Report{}, err
	}
	defer s.Close()
	cfg := config.Default()
	if err = staleMemory(ctx, s, root, cfg); err != nil {
		return Report{}, err
	}
	return RunSuite(ctx, s, root, cfg, builtinSuite, "builtin")
}

// staleMemory saves a sourced memory, then edits the cited file.
func staleMemory(ctx context.Context, s *sqlite.Store, root string, cfg config.Config) error {
	in := memory.Input{Title: "Refund limit", Body: "Refunds cannot exceed the captured amount.", Scope: "payments", SourcePaths: []string{"payments/refund.go"}}
	if _, err := memory.Remember(ctx, s, root, cfg, in); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(root, "payments", "refund.go"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	_, err = f.WriteString("\n// Refund limits are now configurable per merchant.\n")
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

func copyFixture(dir string) error {
	old := time.Now().Add(-time.Hour) // settled mtimes, as in a real checkout
	return fs.WalkDir(fixture, fixtureRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(path, fixtureRoot), "/")
		rel = strings.TrimSuffix(rel, ".txt") // go.mod is stored as go.mod.txt
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		b, err := fixture.ReadFile(path)
		if err != nil {
			return err
		}
		if err = os.WriteFile(target, b, 0600); err != nil {
			return fmt.Errorf("copy fixture: %w", err)
		}
		return os.Chtimes(target, old, old)
	})
}
