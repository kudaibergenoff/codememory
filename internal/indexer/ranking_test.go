package indexer_test

import (
	"testing"

	"github.com/kudaibergenoff/codememory/internal/contextpack"
)

// compoundFixture holds a chunk that names the snake_case identifier and a
// decoy whose symbol column matches a query word, which BM25 weighs 8x.
func compoundFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.write("dash.go", "package dash\n\n// Handler serves memory usage and request context.\nfunc Handler() {}\n")
	f.write("server.go", "package mcp\n\nvar tools = []string{\"memory_context\", \"memory_search\"}\n")
	return f
}

func TestSearchRanksExactCompoundIdentifierFirst(t *testing.T) {
	f := compoundFixture(t)

	got := f.search("memory_context handler")

	if len(got.Matches) == 0 || got.Matches[0].Path != "server.go" {
		t.Fatalf("matches %+v", got.Matches)
	}
}

func TestContextRanksExactCompoundIdentifierFirst(t *testing.T) {
	f := compoundFixture(t)

	got, err := contextpack.Build(f.ctx, f.s, f.root, f.cfg, contextpack.Input{Task: "fix the memory_context handler"})

	if err != nil || len(got.Files) == 0 || got.Files[0] != "server.go" {
		t.Fatalf("files %v %v", got.Files, err)
	}
}

func TestSpacedWordsGetNoExactBoost(t *testing.T) {
	f := compoundFixture(t)

	got := f.search("memory context handler")

	if len(got.Matches) == 0 || got.Matches[0].Path != "dash.go" {
		t.Fatalf("matches %+v", got.Matches)
	}
}
