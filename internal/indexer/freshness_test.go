package indexer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/indexer"
	"github.com/kudaibergenoff/codememory/internal/search"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

type fixture struct {
	t    *testing.T
	ctx  context.Context
	root string
	cfg  config.Config
	s    *sqlite.Store
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	if err := config.Init(root); err != nil {
		t.Fatal(err)
	}
	s, err := sqlite.Open(context.Background(), filepath.Join(root, ".codememory"), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return &fixture{t: t, ctx: context.Background(), root: root, cfg: config.Default(), s: s}
}

// write creates a file whose mtime is old enough to be trusted by the prefilter.
func (f *fixture) write(name, body string) {
	f.t.Helper()
	p := filepath.Join(f.root, name)
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		f.t.Fatal(err)
	}
	f.age(name, time.Hour)
}

func (f *fixture) age(name string, by time.Duration) {
	f.t.Helper()
	old := time.Now().Add(-by)
	if err := os.Chtimes(filepath.Join(f.root, name), old, old); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) index() sqlite.IndexResult {
	f.t.Helper()
	res, err := indexer.Index(f.ctx, f.s, f.root, f.cfg)
	if err != nil {
		f.t.Fatal(err)
	}
	return res
}

func (f *fixture) search(q string) search.Response {
	f.t.Helper()
	res, err := search.Query(f.ctx, f.s, f.root, f.cfg, search.Input{Query: q})
	if err != nil {
		f.t.Fatal(err)
	}
	return res
}

func (f *fixture) status() search.Response {
	f.t.Helper()
	res, err := search.Status(f.ctx, f.s, f.root, f.cfg)
	if err != nil {
		f.t.Fatal(err)
	}
	return res
}

func TestSearchSeesEditsWithoutExplicitIndex(t *testing.T) {
	f := newFixture(t)
	f.write("pay.go", "package pay\nfunc Payment() {}\n")
	f.index()

	f.write("pay.go", "package pay\nfunc Refund() {}\n")
	f.write("new.go", "package pay\nfunc Invoice() {}\n")

	if got := f.search("Refund"); got.Stale || len(got.Matches) != 1 {
		t.Fatalf("edited file not refreshed: %+v", got)
	}
	if got := f.search("Invoice"); len(got.Matches) != 1 {
		t.Fatalf("new file not refreshed: %+v", got)
	}
	if got := f.search("Payment"); len(got.Matches) != 0 {
		t.Fatalf("stale content returned: %+v", got)
	}
}

func TestSearchDropsDeletedFiles(t *testing.T) {
	f := newFixture(t)
	f.write("pay.go", "package pay\nfunc Payment() {}\n")
	f.index()

	if err := os.Remove(filepath.Join(f.root, "pay.go")); err != nil {
		t.Fatal(err)
	}

	if got := f.search("Payment"); len(got.Matches) != 0 {
		t.Fatalf("deleted file returned: %+v", got)
	}
}

func TestStatusReportsStalenessWithoutWriting(t *testing.T) {
	f := newFixture(t)
	f.write("pay.go", "package pay\nfunc Payment() {}\n")
	indexed := f.index()

	f.write("pay.go", "package pay\nfunc Refund() {}\n")
	stale := f.status()

	if !stale.Stale || stale.Generation != indexed.Generation {
		t.Fatalf("status should report stale and not reindex: %+v", stale)
	}
	if again := f.status(); !again.Stale {
		t.Fatal("status wrote to the index")
	}
	f.search("Refund")
	if fresh := f.status(); fresh.Stale {
		t.Fatalf("still stale after refresh: %+v", fresh)
	}
}

func TestNeverIndexedProjectIsStale(t *testing.T) {
	f := newFixture(t)
	f.write("pay.go", "package pay\n")

	if got := f.status(); !got.Stale {
		t.Fatalf("empty index reported fresh: %+v", got)
	}
}

func TestMtimeOnlyChangeKeepsGeneration(t *testing.T) {
	f := newFixture(t)
	f.write("pay.go", "package pay\nfunc Payment() {}\n")
	first := f.index()

	f.age("pay.go", 30*time.Minute) // touch: new mtime, same bytes
	second := f.index()

	if second.Generation != first.Generation || second.Indexed != 0 || second.Reused != 1 {
		t.Fatalf("touch reindexed: %+v", second)
	}
	prior, _, err := f.s.Prior(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(f.root, "pay.go"))
	if err != nil {
		t.Fatal(err)
	}
	if prior["pay.go"].MtimeNS != info.ModTime().UnixNano() {
		t.Fatalf("stat not refreshed: %+v", prior["pay.go"])
	}
}

func TestNewBinaryFileDoesNotAdvanceGeneration(t *testing.T) {
	f := newFixture(t)
	f.write("pay.go", "package pay\n")
	first := f.index()

	f.write("logo.txt", "\x00\x01\x02")
	second := f.index()

	if second.Generation != first.Generation || second.Skipped == 0 {
		t.Fatalf("binary changed generation: %+v", second)
	}
	prior, _, err := f.s.Prior(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if prior["logo.txt"].Skip == "" {
		t.Fatalf("binary skip not cached: %+v", prior)
	}
	if got := f.status(); got.Stale {
		t.Fatalf("binary file made index stale: %+v", got)
	}
}

func TestFileBecomingSecretIsRemoved(t *testing.T) {
	f := newFixture(t)
	f.write("conf.py", "name = 'ok'\n")
	f.index()

	f.write("conf.py", "api_key = \"abcdefgh12345678\"\n")
	got := f.index()

	if got.Deleted != 1 || got.Secrets != 1 {
		t.Fatalf("secret file kept: %+v", got)
	}
}

func TestRefreshFailureWithholdsResultsInsteadOfFailing(t *testing.T) {
	f := newFixture(t)
	f.write("pay.go", "package pay\nfunc Payment() {}\n")
	f.index()
	f.write("more.go", "package pay\nfunc More() {}\n")
	tight := f.cfg
	tight.MaxFiles = 1 // the refresh scan now fails

	got, err := search.Query(f.ctx, f.s, f.root, tight, search.Input{Query: "Payment"})

	if err != nil {
		t.Fatalf("search should degrade, not fail: %v", err)
	}
	if !got.Stale || len(got.Matches) != 0 || len(got.Warnings) == 0 {
		t.Fatalf("expected withheld stale response: %+v", got)
	}
}

func TestFullIndexDetectsChangeWithPreservedMtime(t *testing.T) {
	f := newFixture(t)
	f.write("pay.go", "package pay\nfunc Payment() {}\n")
	f.index()

	path := filepath.Join(f.root, "pay.go")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Same size, mtime restored (cp -p, rsync -t): the prefilter cannot see it.
	if err = os.WriteFile(path, []byte("package pay\nfunc Refunds() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	if got := f.index(); got.Indexed != 0 {
		t.Fatalf("prefilter limitation changed; update docs: %+v", got)
	}

	full, err := indexer.IndexFull(f.ctx, f.s, f.root, f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if full.Indexed != 1 || len(f.search("Refunds").Matches) != 1 {
		t.Fatalf("full index missed the change: %+v", full)
	}
}
