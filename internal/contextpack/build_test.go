package contextpack_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kudaibergenoff/codememory/internal/analytics"
	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/contextpack"
	"github.com/kudaibergenoff/codememory/internal/indexer"
	"github.com/kudaibergenoff/codememory/internal/search"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

var paymentFixtures = []string{"go/payments.go", "php/PaymentController.php", "python/payments.py", "typescript/payments.ts"}

func put(t *testing.T, root, name string, body []byte) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, body, 0600); err != nil {
		t.Fatal(err)
	}
}

// project copies the payment fixtures and adds unrelated files as noise.
func project(t *testing.T) (string, *sqlite.Store) {
	t.Helper()
	root := t.TempDir()
	if err := config.Init(root); err != nil {
		t.Fatal(err)
	}
	for _, f := range paymentFixtures {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", f))
		if err != nil {
			t.Fatal(err)
		}
		put(t, root, f, b)
	}
	for i := 0; i < 20; i++ {
		put(t, root, fmt.Sprintf("noise/n%02d.go", i), []byte(fmt.Sprintf("package noise\n\n// Render draws widget %d.\nfunc Render%d(w int) int { return w * %d }\n", i, i, i)))
	}
	put(t, root, "README.md", []byte("# Shop\nWidgets and an endpoint list.\n"))
	s, err := sqlite.Open(context.Background(), filepath.Join(root, ".codememory"), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err = indexer.Index(context.Background(), s, root, config.Default()); err != nil {
		t.Fatal(err)
	}
	return root, s
}

func build(t *testing.T, root string, s *sqlite.Store, cfg config.Config, in contextpack.Input) contextpack.Response {
	t.Helper()
	got, err := contextpack.Build(context.Background(), s, root, cfg, in)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestRelevantFilesRankInTopFive(t *testing.T) {
	root, s := project(t)

	got := build(t, root, s, config.Default(), contextpack.Input{Task: "Add a refund endpoint for payments"})

	top := map[string]bool{}
	for i, f := range got.Files {
		if i < 5 {
			top[f] = true
		}
	}
	for _, f := range paymentFixtures {
		if !top[f] {
			t.Errorf("%s not in top 5: %v", f, got.Files)
		}
	}
	if got.Stale || len(got.Snippets) == 0 || got.TokenBudget != 1500 {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestEncodedResponseStaysWithinBudget(t *testing.T) {
	root, s := project(t)
	for _, budget := range []int{300, 600, 1500} {
		got := build(t, root, s, config.Default(), contextpack.Input{Task: "refund payment widget render endpoint", MaxTokens: budget})

		b, err := search.Encode(got)
		if err != nil {
			t.Fatal(err)
		}
		if est := analytics.Estimate(len(b)); est > budget || got.EstimatedTokens < est {
			t.Fatalf("budget %d: encoded %d tokens, reported %d", budget, est, got.EstimatedTokens)
		}
	}
}

func TestContextReflectsEditsWithoutIndex(t *testing.T) {
	root, s := project(t)
	put(t, root, "go/chargeback.go", []byte("package payments\n\nfunc OpenChargeback() {}\n"))

	got := build(t, root, s, config.Default(), contextpack.Input{Task: "open chargeback"})

	if len(got.Files) == 0 || got.Files[0] != "go/chargeback.go" {
		t.Fatalf("new file not found first: %v", got.Files)
	}
}

func TestContextWithheldWhenRefreshFails(t *testing.T) {
	root, s := project(t)
	tight := config.Default()
	tight.MaxFiles = 1

	got := build(t, root, s, tight, contextpack.Input{Task: "refund payment"})

	if !got.Stale || len(got.Snippets) != 0 || len(got.Files) != 0 {
		t.Fatalf("content served after failed refresh: %+v", got)
	}
}

func TestInvalidBudgetRejected(t *testing.T) {
	root, s := project(t)
	for _, budget := range []int{-1, 10, 100000} {
		if _, err := contextpack.Build(context.Background(), s, root, config.Default(), contextpack.Input{Task: "refund", MaxTokens: budget}); err == nil {
			t.Errorf("budget %d accepted", budget)
		}
	}
}
