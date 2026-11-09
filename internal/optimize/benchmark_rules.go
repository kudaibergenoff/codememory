package optimize

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kudaibergenoff/codememory/internal/benchmark"
)

// benchmarkRules derive suggestions from the latest benchmark, or ask for one.
func benchmarkRules(e Evidence) []Suggestion {
	if e.Benchmark == nil {
		return []Suggestion{{ID: "no-benchmark", Kind: KindSuggest,
			Evidence: "No benchmark has been run for this project.",
			Action:   "Write a few tasks with known answer files and run codememory benchmark --tasks tasks.json; the built-in suite only covers a fixture."}}
	}
	var out []Suggestion
	var ctx, oracle []int
	var missed []string
	negative := 0
	for _, r := range e.Benchmark.Results {
		if r.Kind != benchmark.KindPositive {
			continue
		}
		ctx, oracle = append(ctx, r.ContextTokens), append(oracle, r.OracleTokens)
		missed = append(missed, r.Missed...)
		if r.SavingsPct != nil && *r.SavingsPct < 0 {
			negative++
		}
	}
	if len(ctx) == 0 {
		return nil
	}
	medCtx, medOracle := median(ctx), median(oracle)
	if e.Budget > 0 && float64(medCtx) >= saturationShare*float64(e.Budget) && float64(medOracle) < oracleShareOfCtx*float64(medCtx) {
		out = append(out, Suggestion{ID: "budget-saturation", Kind: KindSuggest,
			Evidence: fmt.Sprintf("Median context %d tokens fills the %d budget while the expected files total about %d.", medCtx, e.Budget, medOracle),
			Action:   fmt.Sprintf("For navigation tasks pass a smaller max_tokens (e.g. %d); lower-ranked snippets are padding the response.", smallerBudgetHint)})
	}
	if len(missed) > 0 {
		out = append(out, Suggestion{ID: "recall-misses", Kind: KindSuggest,
			Evidence: "Expected files not returned: " + strings.Join(missed, ", ") + ".",
			Action:   "Mention identifiers from those files in tasks, or save a sourced memory (codememory remember --source ...) for that area."})
	}
	if negative > 0 {
		out = append(out, Suggestion{ID: "negative-savings", Kind: KindInfo,
			Evidence: fmt.Sprintf("%d of %d tasks returned more estimated tokens than reading every matching file.", negative, len(ctx)),
			Action:   "Expected on very small projects: response overhead dominates. Prefer direct reads there; CodeMemory pays off as the codebase grows."})
	}
	return out
}

func median(v []int) int {
	s := append([]int(nil), v...)
	sort.Ints(s)
	return s[len(s)/2]
}
