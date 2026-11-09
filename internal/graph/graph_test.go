package graph

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

var shop = map[string]string{
	"go.mod": "module example.com/shop\n",
	"store/store.go": `package store

type Store struct{}

func Open() *Store { return &Store{} }

func New() *Store { return Open() }

func (s *Store) Items() []string { return nil }
`,
	"cart/cart.go": `package cart

import "example.com/shop/store"

func New() []string {
	s := store.Open()
	return s.Items()
}

func Total() int { return len(New()) }
`,
	"cart/cart_test.go": "package cart\n\nimport \"testing\"\n\nfunc TestNew(t *testing.T) { New() }\n",
	"api/api.go": `package api

import (
	"net/http"

	"example.com/shop/cart"
)

func Handle(w http.ResponseWriter) int {
	cart.New()
	return cart.Total()
}
`,
}

type fixture struct {
	ctx  context.Context
	s    *sqlite.Store
	root string
}

func newShop(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	if err := config.Init(root); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	for name, body := range shop {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	s, err := sqlite.Open(context.Background(), filepath.Join(root, ".codememory"), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return fixture{ctx: context.Background(), s: s, root: root}
}

func (f fixture) describe(t *testing.T, in Input) Response {
	t.Helper()
	got, err := Describe(f.ctx, f.s, f.root, config.Default(), in)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func callerList(cs []Caller) string {
	var out []string
	for _, c := range cs {
		s := c.Symbol + "@" + string(rune('0'+c.Depth))
		if c.ByName {
			s += "~"
		}
		if c.Test {
			s += "(test)"
		}
		out = append(out, s)
	}
	return strings.Join(out, " ")
}

func TestCallersFollowCallsAcrossPackagesToDepth(t *testing.T) {
	f := newShop(t)

	got := f.describe(t, Input{Symbol: "store.Open", Depth: 3})

	if len(got.Definitions) != 1 || got.Definitions[0] != (Node{Symbol: "store.Open", Kind: "func", Path: "store/store.go", Line: 5}) {
		t.Fatalf("definitions %+v", got.Definitions)
	}
	want := "cart.New@1 store.New@1 api.Handle@2 cart.TestNew@2(test) cart.Total@2"
	if callerList(got.Callers) != want {
		t.Fatalf("callers\n got %s\nwant %s", callerList(got.Callers), want)
	}
	if got.Callers[0].Path != "cart/cart.go" || got.Callers[0].Line != 6 {
		t.Fatalf("call site %+v", got.Callers[0])
	}
}

func TestDepthDefaultsToDirectCallers(t *testing.T) {
	f := newShop(t)

	got := f.describe(t, Input{Symbol: "store.Open"})

	if callerList(got.Callers) != "cart.New@1 store.New@1" {
		t.Fatalf("callers %s", callerList(got.Callers))
	}
}

func TestMethodCallersAreMatchedByNameAndMarked(t *testing.T) {
	f := newShop(t)

	got := f.describe(t, Input{Symbol: "Store.Items"})

	if len(got.Definitions) != 1 || got.Definitions[0].Symbol != "store.Store.Items" || got.Definitions[0].Kind != "method" {
		t.Fatalf("definitions %+v", got.Definitions)
	}
	if callerList(got.Callers) != "cart.New@1~" {
		t.Fatalf("callers %s", callerList(got.Callers))
	}
}

func TestCalleesResolveInternalExternalAndMethods(t *testing.T) {
	f := newShop(t)

	got := f.describe(t, Input{Symbol: "api.Handle"})

	var list []string
	for _, c := range got.Callees {
		s := c.Symbol
		switch {
		case c.External:
			s += "(ext)"
		case c.Path != "":
			s += "@" + c.Path
		}
		list = append(list, s)
	}
	if strings.Join(list, " ") != "net/http.ResponseWriter(ext) cart.New@cart/cart.go cart.Total@cart/cart.go" {
		t.Fatalf("callees %v", list)
	}
	items := f.describe(t, Input{Symbol: "cart.New"}).Callees
	if len(items) != 2 || items[1].Symbol != "Items" || items[1].Kind != "method" || items[1].Candidates != 1 {
		t.Fatalf("method callee %+v", items)
	}
}

func TestHubsRankMostReferencedSymbolsWithoutTests(t *testing.T) {
	f := newShop(t)

	got := f.describe(t, Input{})

	if len(got.Hubs) == 0 || got.Hubs[0].Symbol != "cart.New" || got.Hubs[0].Callers != 2 || got.Hubs[0].Path != "cart/cart.go" {
		t.Fatalf("hubs %+v", got.Hubs)
	}
	scoped := f.describe(t, Input{Scope: "store"})
	for _, h := range scoped.Hubs {
		if !strings.HasPrefix(h.Path, "store/") {
			t.Fatalf("hub outside scope: %+v", h)
		}
	}
}

func TestAmbiguousAndUnknownSymbolsAreReported(t *testing.T) {
	f := newShop(t)

	amb := f.describe(t, Input{Symbol: "New"})
	unknown := f.describe(t, Input{Symbol: "store.Missing"})

	if len(amb.Definitions) != 2 || len(amb.Callers) != 0 || !hasWarning(amb.Warnings, "ambiguous") {
		t.Fatalf("ambiguous %+v", amb)
	}
	if len(unknown.Definitions) != 0 || !hasWarning(unknown.Warnings, "not found") {
		t.Fatalf("unknown %+v", unknown)
	}
}

func TestInvalidInputRejected(t *testing.T) {
	f := newShop(t)
	for _, in := range []Input{
		{Symbol: "a.b.c.d"}, {Symbol: "store..Open"}, {Symbol: "x; DROP"}, {Symbol: "store.Open", Depth: 4},
		{Symbol: "store.Open", Depth: -1}, {Scope: "../x"},
	} {
		if _, err := Describe(f.ctx, f.s, f.root, config.Default(), in); err == nil {
			t.Errorf("accepted %+v", in)
		}
	}
}

func hasWarning(ws []string, word string) bool {
	for _, w := range ws {
		if strings.Contains(strings.ToLower(w), word) {
			return true
		}
	}
	return false
}

func TestBuildTaggedDuplicatesAreOneDefinition(t *testing.T) {
	f := newShop(t)
	alt := filepath.Join(f.root, "store", "open_windows.go")
	if err := os.WriteFile(alt, []byte("//go:build windows\n\npackage store\n\nfunc Open() *Store { return nil }\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := f.describe(t, Input{Symbol: "store.Open"})
	hubs := f.describe(t, Input{}).Hubs

	if len(got.Definitions) != 1 || got.Definitions[0].Path != "store/open_windows.go" || len(got.Callers) != 2 {
		t.Fatalf("definitions %+v callers %+v", got.Definitions, got.Callers)
	}
	seen := map[string]bool{}
	for _, h := range hubs {
		if seen[h.Symbol] {
			t.Fatalf("duplicate hub %s in %+v", h.Symbol, hubs)
		}
		seen[h.Symbol] = true
	}
}

func (f fixture) write(t *testing.T, name, body string) {
	t.Helper()
	p := filepath.Join(f.root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDirSelectsOneOfSameNamedPackages(t *testing.T) {
	f := newShop(t)

	got := f.describe(t, Input{Symbol: "New", Dir: "cart"})

	if len(got.Definitions) != 1 || got.Definitions[0].Path != "cart/cart.go" || len(got.Callers) == 0 {
		t.Fatalf("%+v", got)
	}
	if _, err := Describe(f.ctx, f.s, f.root, config.Default(), Input{Symbol: "New", Dir: "../x"}); err == nil {
		t.Fatal("accepted dir outside the project")
	}
}

func TestDefinitionsAreCapped(t *testing.T) {
	f := newShop(t)
	var b strings.Builder
	b.WriteString("package many\n\n")
	for i := 0; i < maxDefinitions+5; i++ {
		fmt.Fprintf(&b, "type T%d struct{}\n\nfunc (T%d) String() string { return \"\" }\n\n", i, i)
	}
	f.write(t, "many/many.go", b.String())

	got := f.describe(t, Input{Symbol: "String"})

	if len(got.Definitions) != maxDefinitions || !got.Truncated {
		t.Fatalf("%d definitions, truncated %v", len(got.Definitions), got.Truncated)
	}
}

func TestCallersAreCappedAndCalleesSkipConstants(t *testing.T) {
	f := newShop(t)
	var b strings.Builder
	b.WriteString("package fan\n\nimport \"example.com/shop/store\"\n\n")
	for i := 0; i < maxCallers+10; i++ {
		fmt.Fprintf(&b, "func F%d() { store.Open() }\n\n", i)
	}
	b.WriteString("const (\n")
	for i := 0; i < maxCallees+10; i++ {
		fmt.Fprintf(&b, "\tC%d = %d\n", i, i)
	}
	b.WriteString(")\n\nfunc Heavy() int {\n\tn := 0\n")
	for i := 0; i < maxCallees+10; i++ {
		fmt.Fprintf(&b, "\tn += C%d\n", i)
	}
	b.WriteString("\tstore.Open()\n\treturn n\n}\n")
	f.write(t, "fan/fan.go", b.String())

	open := f.describe(t, Input{Symbol: "store.Open", Depth: 3})
	heavy := f.describe(t, Input{Symbol: "fan.Heavy"})

	if len(open.Callers) != maxCallers || !open.Truncated {
		t.Fatalf("%d callers, truncated %v", len(open.Callers), open.Truncated)
	}
	if len(heavy.Callees) != 1 || heavy.Callees[0].Symbol != "store.Open" || heavy.Truncated {
		t.Fatalf("callees %+v truncated %v", heavy.Callees, heavy.Truncated)
	}
}

func TestBareNamePrefersFunctionsAndTypesOverMethods(t *testing.T) {
	f := newShop(t)
	f.write(t, "store/reopen.go", "package store\n\nfunc (s *Store) Open() *Store { return s }\n")

	got := f.describe(t, Input{Symbol: "Open", Dir: "store"})
	items := f.describe(t, Input{Symbol: "Items"})

	if len(got.Definitions) != 1 || got.Definitions[0].Kind != "func" {
		t.Fatalf("definitions %+v", got.Definitions)
	}
	if len(items.Definitions) != 1 || items.Definitions[0].Kind != "method" {
		t.Fatalf("method-only name lost: %+v", items.Definitions)
	}
}
