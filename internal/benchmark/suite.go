package benchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/kudaibergenoff/codememory/internal/memory"
	"github.com/kudaibergenoff/codememory/internal/search"
)

const (
	maxTasks     = 200
	maxIDBytes   = 64
	maxSuiteSize = 1 << 20
)

// Task is one retrieval scenario. Exactly one expectation must be set.
type Task struct {
	ID                 string   `json:"id"`
	Task               string   `json:"task"`
	ExpectFiles        []string `json:"expect_files,omitempty"`
	ExpectNoMatch      bool     `json:"expect_no_match,omitempty"`
	ExpectMemoryStatus string   `json:"expect_memory_status,omitempty"`
	MaxTokens          int      `json:"max_tokens,omitempty"`
}

type Suite struct {
	Tasks []Task `json:"tasks"`
}

// LoadSuite reads and validates a task file.
func LoadSuite(r io.Reader) (Suite, error) {
	dec := json.NewDecoder(io.LimitReader(r, maxSuiteSize))
	dec.DisallowUnknownFields()
	var s Suite
	if err := dec.Decode(&s); err != nil {
		return Suite{}, fmt.Errorf("read tasks: %w", err)
	}
	if len(s.Tasks) == 0 || len(s.Tasks) > maxTasks {
		return Suite{}, fmt.Errorf("tasks must contain 1 to %d entries", maxTasks)
	}
	seen := map[string]bool{}
	for _, t := range s.Tasks {
		if err := t.validate(); err != nil {
			return Suite{}, fmt.Errorf("task %q: %w", t.ID, err)
		}
		if seen[t.ID] {
			return Suite{}, fmt.Errorf("duplicate task id %q", t.ID)
		}
		seen[t.ID] = true
	}
	return s, nil
}

func (t Task) validate() error {
	if t.ID == "" || len(t.ID) > maxIDBytes {
		return fmt.Errorf("id must be 1 to %d bytes", maxIDBytes)
	}
	if t.Task == "" {
		return errors.New("task text is required")
	}
	expectations := 0
	if len(t.ExpectFiles) > 0 {
		expectations++
	}
	if t.ExpectNoMatch {
		expectations++
	}
	if t.ExpectMemoryStatus != "" {
		expectations++
	}
	if expectations != 1 {
		return errors.New("set exactly one of expect_files, expect_no_match, expect_memory_status")
	}
	for _, p := range t.ExpectFiles {
		if err := search.CheckRelative(p); err != nil {
			return err
		}
	}
	switch t.ExpectMemoryStatus {
	case "", memory.StatusSourced, memory.StatusUnverified, memory.StatusStale:
		return nil
	default:
		return fmt.Errorf("expect_memory_status must be %s, %s or %s", memory.StatusSourced, memory.StatusUnverified, memory.StatusStale)
	}
}
