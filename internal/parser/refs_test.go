package parser

import (
	"fmt"
	"strings"
	"testing"
)

const refsSource = `package shop

import (
	"fmt"
	str "strings"
	"example.com/shop/internal/store"
)

type Cart struct{ items []Item }

func New(s *store.Store) *Cart {
	c := &Cart{}
	c.Load(s)
	total := Total(c.items)
	fmt.Println(total, str.ToUpper("x"), len(c.items), Name)
	return c
}

func (c *Cart) Load(s *store.Store) {
	for _, it := range s.Items() {
		c.items = append(c.items, Item{ID: it.ID})
	}
}
`

func refsByFrom(rs []Ref) map[string][]string {
	out := map[string][]string{}
	for _, r := range rs {
		out[r.From] = append(out[r.From], fmt.Sprintf("%s|%s|%s", r.Kind, r.Pkg, r.Name))
	}
	return out
}

func TestRefsRecordPackageLevelUsesPerFunction(t *testing.T) {
	r := Parse("cart.go", "go", []byte(refsSource), 40)

	got := refsByFrom(r.Refs)

	want := map[string][]string{
		"New": {
			"ref|example.com/shop/internal/store|Store", "ref||Cart", "method|.|Load",
			"call||Total", "call|fmt|Println", "call|strings|ToUpper", "ref||Name",
		},
		"Cart.Load": {"ref|example.com/shop/internal/store|Store", "method|.|Items", "ref||Item"},
	}
	for from, refs := range want {
		if strings.Join(got[from], " ") != strings.Join(refs, " ") {
			t.Errorf("%s:\n got %q\nwant %q", from, got[from], refs)
		}
	}
	if len(got) != len(want) {
		t.Errorf("unexpected sources: %v", got)
	}
}

func TestRefsSkipLocalsBuiltinsFieldsAndKeys(t *testing.T) {
	r := Parse("cart.go", "go", []byte(refsSource), 40)

	for _, ref := range r.Refs {
		switch ref.Name {
		case "c", "s", "total", "it", "len", "append", "items", "ID", "New":
			t.Errorf("recorded %+v", ref)
		}
	}
}

func TestRefsAreDeduplicatedWithFirstLineAndUses(t *testing.T) {
	src := "package p\n\nfunc A() {\n\tB()\n\tB()\n\t_ = B\n}\n\nfunc B() {}\n"

	r := Parse("p.go", "go", []byte(src), 40)

	if len(r.Refs) != 1 || r.Refs[0] != (Ref{From: "A", Name: "B", Kind: RefCall, Line: 4, Uses: 3}) {
		t.Fatalf("%+v", r.Refs)
	}
}

func TestMethodSymbolsCarryTheirReceiver(t *testing.T) {
	src := "package p\n\ntype S struct{}\n\nfunc (s *S) Run() {}\n\nfunc (S) Stop() {}\n\nfunc (g G[T]) Get() {}\n"

	r := Parse("p.go", "go", []byte(src), 40)

	var methods []string
	for _, s := range r.Symbols {
		if s.Kind == "method" {
			methods = append(methods, s.Name)
		}
	}
	if strings.Join(methods, " ") != "S.Run S.Stop G.Get" {
		t.Fatalf("%v", methods)
	}
}

func TestImportNames(t *testing.T) {
	cases := map[string]string{
		"fmt":                    "fmt",
		"example.com/shop/store": "store",
		"gopkg.in/yaml.v3":       "yaml",
		"github.com/x/mod/v2":    "mod",
		"github.com/modelcontextprotocol/go-sdk/mcp": "mcp",
	}
	for path, want := range cases {
		if got := importName(path); got != want {
			t.Errorf("%s: got %s, want %s", path, got, want)
		}
	}
}

func TestNonGoAndBrokenFilesHaveNoRefs(t *testing.T) {
	if r := Parse("a.py", "python", []byte("def f(): g()\n"), 40); len(r.Refs) != 0 {
		t.Fatalf("%+v", r.Refs)
	}
	if r := Parse("a.go", "go", []byte("package p\nfunc A() { B( }\n"), 40); len(r.Refs) != 0 {
		t.Fatalf("%+v", r.Refs)
	}
}

func TestRefsReviewCases(t *testing.T) {
	src := `package p

func min(a, b int) int { return a }

func (*T) M() {}

func A() {
	_ = map[Kind]int{KindA: 1}
	_ = S{Field: 1}
	_ = min(1, 2)
	Gen[int](1)
}
`
	r := Parse("p.go", "go", []byte(src), 40)

	got := strings.Join(refsByFrom(r.Refs)["A"], " ")
	if got != "ref||Kind ref||KindA ref||S call||min call||Gen" {
		t.Fatalf("A refs: %s", got)
	}
	var methods []string
	for _, s := range r.Symbols {
		if s.Kind == "method" {
			methods = append(methods, s.Name)
		}
	}
	if strings.Join(methods, " ") != "T.M" {
		t.Fatalf("methods %v", methods)
	}
}
