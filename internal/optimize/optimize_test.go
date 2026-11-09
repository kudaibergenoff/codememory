package optimize

import (
	"strings"
	"testing"

	"github.com/kudaibergenoff/codememory/internal/analytics"
	"github.com/kudaibergenoff/codememory/internal/benchmark"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

func ids(s []Suggestion) map[string]bool {
	out := map[string]bool{}
	for _, x := range s {
		out[x.ID] = true
	}
	return out
}

func f(v float64) *float64 { return &v }

func healthyBenchmark() *benchmark.Report {
	return &benchmark.Report{Results: []benchmark.Result{
		{ID: "a", Kind: benchmark.KindPositive, Pass: true, Recall: f(1), ContextTokens: 700, OracleTokens: 600, SavingsPct: f(80)},
	}}
}

func TestHealthyProjectGetsNoActionableSuggestions(t *testing.T) {
	got := Suggest(Evidence{Sizes: []sqlite.PathSize{{Path: "a/x.go", Size: 100}, {Path: "b/y.go", Size: 100}}, Benchmark: healthyBenchmark(), Budget: 1500})

	for _, s := range got {
		if s.Kind == KindSuggest {
			t.Fatalf("unexpected suggestion %+v", s)
		}
	}
}

func TestRulesFireOnEvidence(t *testing.T) {
	big := []sqlite.PathSize{{Path: "vendor_copy/lib/a.js", Size: 900 << 10}, {Path: "src/main.go", Size: 100 << 10}}
	bench := &benchmark.Report{Results: []benchmark.Result{
		{ID: "a", Kind: benchmark.KindPositive, Recall: f(0.5), Missed: []string{"src/pay.go"}, ContextTokens: 1490, OracleTokens: 200, SavingsPct: f(-40)},
		{ID: "b", Kind: benchmark.KindPositive, Recall: f(1), ContextTokens: 1480, OracleTokens: 300, SavingsPct: f(-10)},
	}}
	e := Evidence{
		Sizes:         big,
		Skips:         map[string]int{"secret": 2},
		Stats:         analytics.Report{Measured: analytics.Measured{Requests: 10, Errors: 3}},
		Benchmark:     bench,
		StaleMemories: 1,
		Budget:        1500,
	}

	got := ids(Suggest(e))

	for _, want := range []string{"large-directory", "secret-skips", "budget-saturation", "recall-misses", "tool-errors", "stale-memories", "negative-savings"} {
		if !got[want] {
			t.Errorf("rule %s did not fire: %v", want, got)
		}
	}
}

func TestMissingBenchmarkIsSuggested(t *testing.T) {
	got := Suggest(Evidence{Budget: 1500})

	if !ids(got)["no-benchmark"] {
		t.Fatalf("%+v", got)
	}
	for _, s := range got {
		if strings.TrimSpace(s.Evidence) == "" || strings.TrimSpace(s.Action) == "" {
			t.Fatalf("suggestion without evidence or action: %+v", s)
		}
	}
}
