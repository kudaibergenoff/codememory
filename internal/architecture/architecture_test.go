package architecture_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kudaibergenoff/codememory/internal/architecture"
	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/memory"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

type shop struct {
	t    *testing.T
	root string
	s    *sqlite.Store
}

func newShop(t *testing.T) *shop {
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
	sh := &shop{t: t, root: root, s: s}
	sh.write("go.mod", "module example.com/shop\n\ngo 1.27\n")
	sh.write("internal/store/store.go", "package store\n\nfunc Save() {}\n")
	sh.write("internal/pay/pay.go", "package pay\n\nimport (\n\t\"fmt\"\n\t\"example.com/shop/internal/store\"\n)\n\nfunc Pay() { fmt.Println(); store.Save() }\n")
	sh.write("internal/pay/pay_test.go", "package pay_test\n\nimport (\n\t\"testing\"\n\t\"example.com/shop/internal/pay\"\n)\n\nfunc TestPay(t *testing.T) { pay.Pay() }\n")
	sh.write("web/app.ts", "export function pay() {}\n")
	return sh
}

func (sh *shop) write(name, body string) {
	sh.t.Helper()
	p := filepath.Join(sh.root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		sh.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		sh.t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		sh.t.Fatal(err)
	}
}

func (sh *shop) describe(scope string) architecture.Response {
	sh.t.Helper()
	got, err := architecture.Describe(context.Background(), sh.s, sh.root, config.Default(), architecture.Input{Scope: scope})
	if err != nil {
		sh.t.Fatal(err)
	}
	return got
}

func modulesByPath(r architecture.Response) map[string]architecture.Module {
	out := map[string]architecture.Module{}
	for _, m := range r.Modules {
		out[m.Path] = m
	}
	return out
}

func TestModulesAndResolvedImports(t *testing.T) {
	sh := newShop(t)

	got := sh.describe("")

	mods := modulesByPath(got)
	pay := mods["internal/pay"]
	// Test files count as files but never define the package or its imports.
	if got.GoModule != "example.com/shop" || pay.Package != "pay" || pay.Files != 2 {
		t.Fatalf("pay module: %+v (go module %q)", pay, got.GoModule)
	}
	if len(pay.Imports) != 1 || pay.Imports[0] != "internal/store" || pay.ExternalImports != 1 {
		t.Fatalf("imports: %+v", pay)
	}
	if web := mods["web"]; len(web.Languages) != 1 || web.Languages[0] != "typescript" || web.Package != "" {
		t.Fatalf("web module: %+v", web)
	}
	if _, ok := mods["."]; !ok {
		t.Fatalf("root module missing: %v", mods)
	}
}

func TestScopeFiltersModulesAndMemories(t *testing.T) {
	sh := newShop(t)
	for _, in := range []memory.Input{
		{Title: "Pay via store", Body: "pay must persist through store.Save.", Scope: "internal/pay", SourcePaths: []string{"internal/pay/pay.go"}},
		{Title: "Web", Body: "Web calls the API only.", Scope: "web"},
	} {
		if _, err := memory.Remember(context.Background(), sh.s, sh.root, config.Default(), in); err != nil {
			t.Fatal(err)
		}
	}

	got := sh.describe("internal")

	for _, m := range got.Modules {
		if m.Path != "internal/pay" && m.Path != "internal/store" {
			t.Fatalf("out-of-scope module %q", m.Path)
		}
	}
	if len(got.Memories) != 1 || got.Memories[0].Title != "Pay via store" || got.Memories[0].Status != memory.StatusSourced {
		t.Fatalf("memories: %+v", got.Memories)
	}
}

func TestEditsAreReflectedAndMemoriesTurnStale(t *testing.T) {
	sh := newShop(t)
	if _, err := memory.Remember(context.Background(), sh.s, sh.root, config.Default(), memory.Input{Title: "No deps", Body: "store has no imports.", Scope: "internal/store", SourcePaths: []string{"internal/store/store.go"}}); err != nil {
		t.Fatal(err)
	}

	sh.write("internal/store/store.go", "package store\n\nimport \"example.com/shop/internal/pay\"\n\nfunc Save() { pay.Pay() }\n")
	got := sh.describe("internal/store")

	store := modulesByPath(got)["internal/store"]
	if len(store.Imports) != 1 || store.Imports[0] != "internal/pay" {
		t.Fatalf("new import not visible: %+v", store)
	}
	if len(got.Memories) != 1 || got.Memories[0].Status != memory.StatusStale {
		t.Fatalf("memory not stale: %+v", got.Memories)
	}
}

func TestInvalidScopeRejected(t *testing.T) {
	sh := newShop(t)
	for _, scope := range []string{"/abs", "../up", "a\\b"} {
		if _, err := architecture.Describe(context.Background(), sh.s, sh.root, config.Default(), architecture.Input{Scope: scope}); err == nil {
			t.Errorf("scope %q accepted", scope)
		}
	}
}
