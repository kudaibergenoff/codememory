package memory

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kudaibergenoff/codememory/internal/scanner"
	"github.com/kudaibergenoff/codememory/internal/search"
)

const (
	maxTitleBytes = 200
	maxBodyBytes  = 4000
	maxScopeBytes = 128
	maxSources    = 20
)

// normalize validates input at the trust boundary and returns a cleaned copy.
func normalize(in Input) (Input, error) {
	out := Input{Title: strings.TrimSpace(in.Title), Body: strings.TrimSpace(in.Body), Scope: normalizeScope(in.Scope)}
	if err := checkText("title", out.Title, maxTitleBytes, false); err != nil {
		return Input{}, err
	}
	if err := checkText("body", out.Body, maxBodyBytes, true); err != nil {
		return Input{}, err
	}
	if out.Scope == "" {
		out.Scope = DefaultScope
	}
	if err := checkScope(out.Scope); err != nil {
		return Input{}, err
	}
	text := []byte(out.Title + "\n" + out.Body + "\n" + out.Scope)
	if scanner.ContainsSecret(text) || assignedSecret.Match(text) {
		return Input{}, errors.New("memory text resembles a secret; not saved")
	}
	paths, err := checkPaths(in.SourcePaths)
	if err != nil {
		return Input{}, err
	}
	out.SourcePaths = paths
	return out, nil
}

// assignedSecret is stricter than the file heuristic: memories are re-served
// in every matching context, so unquoted assignments are refused too.
var assignedSecret = regexp.MustCompile(`(?i)\b(?:password|passwd|pwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|client[_-]?secret)\b\s*[:=]\s*\S{8,}`)

// checkScope accepts the global sentinel or a clean project-relative directory.
func checkScope(scope string) error {
	if err := checkText("scope", scope, maxScopeBytes, false); err != nil {
		return err
	}
	if scope == DefaultScope {
		return nil
	}
	if scope == "." || search.CheckRelative(scope) != nil {
		return fmt.Errorf("scope %q must be a clean project-relative directory or %s", scope, DefaultScope)
	}
	return nil
}

func normalizeScope(scope string) string {
	return strings.TrimRight(strings.TrimSpace(scope), "/")
}

func checkText(field, s string, limit int, multiline bool) error {
	if s == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	if len(s) > limit || !utf8.ValidString(s) {
		return fmt.Errorf("%s must be valid UTF-8 and at most %d bytes", field, limit)
	}
	for _, r := range s {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\t')) {
			return fmt.Errorf("%s contains control characters", field)
		}
	}
	return nil
}

// checkPaths accepts clean, project-relative slash paths and removes duplicates.
func checkPaths(paths []string) ([]string, error) {
	if len(paths) > maxSources {
		return nil, fmt.Errorf("at most %d source paths", maxSources)
	}
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if err := search.CheckRelative(p); err != nil {
			return nil, err
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out, nil
}
