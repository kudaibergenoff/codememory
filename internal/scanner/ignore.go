package scanner

import (
	"errors"
	"path"
	"strings"
)

type rule struct {
	base, pattern                string
	negated, directory, anchored bool
}

func parseRules(base, content string) ([]rule, error) {
	var result []rule
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSuffix(line, "\r")
		for strings.HasSuffix(line, " ") && !strings.HasSuffix(line, `\ `) {
			line = strings.TrimSuffix(line, " ")
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) > 1024 {
			return nil, errors.New("ignore pattern exceeds 1024 bytes")
		}
		if strings.Contains(line, "[:") || strings.Contains(line, "[.") || strings.Contains(line, "[=") {
			return nil, errors.New("POSIX character classes and collating expressions are not supported in ignore rules")
		}
		r := rule{base: base}
		if line[0] == '!' {
			r.negated = true
			line = line[1:]
		}
		r.directory = strings.HasSuffix(line, "/")
		line = strings.TrimSuffix(line, "/")
		r.anchored = strings.Contains(line, "/")
		line = strings.TrimPrefix(line, "/")
		if line == "" {
			return nil, errors.New("empty ignore pattern")
		}
		for _, segment := range strings.Split(line, "/") {
			if _, err := path.Match(segment, ""); err != nil {
				return nil, errors.New("invalid ignore pattern")
			}
		}
		r.pattern = line
		// Git's [!abc] negation is spelled [^abc] by path.Match.
		r.pattern = strings.ReplaceAll(r.pattern, "[!", "[^")
		result = append(result, r)
	}
	return result, nil
}

func ignored(rules []rule, name string, isDir bool) bool {
	ignored := false
	for _, r := range rules {
		local := name
		if r.base != "" {
			if !strings.HasPrefix(name, r.base+"/") {
				continue
			}
			local = strings.TrimPrefix(name, r.base+"/")
		}
		if r.directory && !isDir {
			continue
		}
		match := false
		if !r.anchored {
			match, _ = path.Match(r.pattern, path.Base(local))
		} else {
			match = matchSegments(strings.Split(r.pattern, "/"), strings.Split(local, "/"))
		}
		if match {
			ignored = !r.negated
		}
	}
	return ignored
}

// Dynamic programming keeps repeated ** patterns bounded, without regex backtracking.
func matchSegments(pattern, parts []string) bool {
	prev := make([]bool, len(parts)+1)
	prev[0] = true
	for i, p := range pattern {
		next := make([]bool, len(parts)+1)
		if p == "**" {
			next[0] = prev[0]
			for j := 1; j <= len(parts); j++ {
				next[j] = prev[j] || next[j-1]
			}
			// A trailing '/**' excludes descendants, not the directory itself.
			if i == len(pattern)-1 && i > 0 {
				next[0] = false
				for j := 1; j <= len(parts); j++ {
					next[j] = prev[j-1] || next[j-1]
				}
			}
		} else {
			for j := 1; j <= len(parts); j++ {
				matched, _ := path.Match(p, parts[j-1])
				next[j] = prev[j-1] && matched
			}
		}
		prev = next
	}
	return prev[len(parts)]
}
