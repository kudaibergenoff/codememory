package memory

import (
	"errors"
	"fmt"
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
	if err := checkText("scope", out.Scope, maxScopeBytes, false); err != nil {
		return Input{}, err
	}
	if scanner.ContainsSecret([]byte(out.Title + "\n" + out.Body)) {
		return Input{}, errors.New("memory text resembles a secret; not saved")
	}
	paths, err := checkPaths(in.SourcePaths)
	if err != nil {
		return Input{}, err
	}
	out.SourcePaths = paths
	return out, nil
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
