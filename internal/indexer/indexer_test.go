package indexer_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/indexer"
	"github.com/kudaibergenoff/codememory/internal/search"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

func TestIncrementalFreshnessAndRollback(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cfg := config.Default()
	if err := config.Init(root); err != nil {
		t.Fatal(err)
	}
	s, err := sqlite.Open(ctx, filepath.Join(root, ".codememory"), root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("pay.go", "package pay\nfunc Payment() {}\n")
	first, err := indexer.Index(ctx, s, root, cfg)
	if err != nil || first.Indexed != 1 {
		t.Fatalf("%+v %v", first, err)
	}
	var chunkID int64
	if err = s.DB.QueryRow(`SELECT id FROM chunks LIMIT 1`).Scan(&chunkID); err != nil {
		t.Fatal(err)
	}
	second, err := indexer.Index(ctx, s, root, cfg)
	if err != nil || second.Indexed != 0 || second.Reused != 1 || second.Generation != first.Generation {
		t.Fatalf("%+v %v", second, err)
	}
	var after int64
	s.DB.QueryRow(`SELECT id FROM chunks LIMIT 1`).Scan(&after)
	if after != chunkID {
		t.Fatal("no-op recreated chunks")
	}
	res, err := search.Query(ctx, s, root, cfg, search.Input{Query: "Payment"})
	if err != nil || res.Stale || len(res.Matches) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	write("pay.go", "package pay\nfunc Refund() {}\n")
	// Search refreshes the edited file first: the old symbol must not leak.
	res, err = search.Query(ctx, s, root, cfg, search.Input{Query: "Payment"})
	if err != nil || res.Stale || len(res.Matches) != 0 || res.Generation != first.Generation+1 {
		t.Fatalf("stale leak %+v %v", res, err)
	}
	updated, err := indexer.Index(ctx, s, root, cfg)
	if err != nil || updated.Generation != first.Generation+1 || updated.Indexed != 0 {
		t.Fatalf("%+v %v", updated, err)
	}
	write("other.py", "def refund(): pass\n")
	small := cfg
	small.MaxFiles = 1
	if _, err = indexer.Index(ctx, s, root, small); err == nil {
		t.Fatal("partial scan accepted")
	}
	st, _ := s.Status(ctx)
	if st.Generation != updated.Generation || st.Files != 1 {
		t.Fatal("failed scan published")
	}
	if err = os.Remove(filepath.Join(root, "pay.go")); err != nil {
		t.Fatal(err)
	}
	deleted, err := indexer.Index(ctx, s, root, cfg)
	if err != nil || deleted.Deleted != 1 || deleted.Indexed != 1 {
		t.Fatalf("%+v %v", deleted, err)
	}
	write(".gitignore", "*.py\n")
	deleted, err = indexer.Index(ctx, s, root, cfg)
	if err != nil || deleted.Deleted != 1 {
		t.Fatalf("new exclusion %+v %v", deleted, err)
	}
}

func TestConcurrentIndexAndSearch(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := config.Init(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pay.go"), []byte("package pay\nfunc Payment() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := sqlite.Open(ctx, filepath.Join(root, ".codememory"), root)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := sqlite.Open(ctx, filepath.Join(root, ".codememory"), root)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if _, err = indexer.Index(ctx, a, root, config.Default()); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	fail := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				if i%2 == 0 {
					_, err := indexer.Index(ctx, a, root, config.Default())
					if err != nil {
						fail <- err
						return
					}
				} else {
					res, err := search.Query(ctx, b, root, config.Default(), search.Input{Query: "Payment"})
					if err != nil {
						fail <- err
						return
					}
					if res.Stale || len(res.Matches) != 1 {
						t.Errorf("inconsistent snapshot: %+v", res)
						return
					}
				}
			}
		}(i)
	}
	wg.Wait()
	close(fail)
	for err := range fail {
		t.Error(err)
	}
	st, err := a.Status(ctx)
	if err != nil || st.Generation != 1 {
		t.Fatalf("%+v %v", st, err)
	}
}

func TestMultilanguageRetrieval(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := config.Init(root); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []string{"go/payments.go", "php/PaymentController.php", "python/payments.py", "typescript/payments.ts"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", fixture))
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(root, fixture)
		if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(p, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := sqlite.Open(ctx, filepath.Join(root, ".codememory"), root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = indexer.Index(ctx, s, root, config.Default()); err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"go", "php", "python", "typescript"} {
		q := "RefundPayment"
		if language == "python" {
			q = "refund payment"
		}
		result, err := search.Query(ctx, s, root, config.Default(), search.Input{Query: q, Language: language, Limit: 5})
		if err != nil || len(result.Matches) != 1 {
			t.Fatalf("%s: %+v %v", language, result, err)
		}
	}
}
