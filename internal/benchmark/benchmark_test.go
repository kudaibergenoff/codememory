package benchmark

import (
	"context"
	"strings"
	"testing"
)

func TestSavingsFormula(t *testing.T) {
	cases := []struct {
		baseline, actual int
		want             *float64
	}{
		{0, 10, nil},
		{100, 40, ptr(60)},
		{100, 150, ptr(-50)}, // negative savings are reported, not hidden
		{100, 100, ptr(0)},
	}
	for _, c := range cases {
		got := savingsPct(c.baseline, c.actual)
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("savings(%d,%d) = %v, want %v", c.baseline, c.actual, deref(got), deref(c.want))
		}
	}
}

func ptr(v float64) *float64 { return &v }

func deref(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestBuiltinSuitePassesIncludingNegativeCases(t *testing.T) {
	report, err := Builtin(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	kinds := map[string]int{}
	for _, r := range report.Results {
		kinds[r.Kind]++
		if !r.Pass {
			t.Errorf("%s (%s) failed: %+v", r.ID, r.Kind, r)
		}
		if r.Kind == KindPositive && (r.Recall == nil || r.BaselineTokens == 0 || r.ContextTokens == 0) {
			t.Errorf("%s: incomplete measurement %+v", r.ID, r)
		}
	}
	if kinds[KindPositive] < 5 || kinds[KindNoMatch] < 1 || kinds[KindStaleMemory] < 1 {
		t.Fatalf("suite composition %v", kinds)
	}
	if report.VerifiedSavings != nil || report.Summary.MeanRecall == nil || *report.Summary.MeanRecall != 1 {
		t.Fatalf("summary %+v verified=%v", report.Summary, report.VerifiedSavings)
	}
}

func TestLoadSuiteValidation(t *testing.T) {
	bad := []string{
		`not json`,
		`{"tasks":[]}`,
		`{"tasks":[{"id":"","task":"x","expect_files":["a.go"]}]}`,
		`{"tasks":[{"id":"a","task":"","expect_files":["a.go"]}]}`,
		`{"tasks":[{"id":"a","task":"x","expect_files":["/etc/passwd"]}]}`,
		`{"tasks":[{"id":"a","task":"x"}]}`,
		`{"tasks":[{"id":"a","task":"x","expect_files":["a.go"]},{"id":"a","task":"y","expect_files":["b.go"]}]}`,
		`{"tasks":[{"id":"a","task":"x","expect_files":["a.go"],"unknown":1}]}`,
	}
	for _, in := range bad {
		if _, err := LoadSuite(strings.NewReader(in)); err == nil {
			t.Errorf("accepted %s", in)
		}
	}
	ok := `{"tasks":[{"id":"a","task":"refund","expect_files":["pay/refund.go"]},{"id":"b","task":"helm chart","expect_no_match":true}]}`
	if s, err := LoadSuite(strings.NewReader(ok)); err != nil || len(s.Tasks) != 2 {
		t.Fatalf("%+v %v", s, err)
	}
}
