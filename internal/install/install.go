// Package install plans and applies client integration files. Nothing is
// written unless Options.Apply is set; every change is shown as a diff first.
package install

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	ActionCreate    = "create"
	ActionUpdate    = "update"
	ActionUnchanged = "unchanged"

	serverName = "codememory"
)

type Options struct {
	Root       string // canonical project root
	Executable string // absolute path of the codememory binary to register
	CodexHome  string // Codex config directory; default $CODEX_HOME or ~/.codex
	Apply      bool
}

type Change struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	Diff   string `json:"diff,omitempty"`
}

type Plan struct {
	Client  string   `json:"client"`
	Applied bool     `json:"applied"`
	Changes []Change `json:"changes"`
	Next    string   `json:"next,omitempty"`
	Note    string   `json:"note"`
}

// target is one file, addressed inside dir through os.Root so that symlinks
// can never redirect a write outside dir.
type target struct {
	dir, name string
	mode      os.FileMode
	render    func(old []byte, exists bool) ([]byte, error)
}

func (t target) display() string { return filepath.Join(t.dir, filepath.FromSlash(t.name)) }

type planned struct {
	target
	content []byte
	action  string
}

// run renders every target before writing any, so a rendering error never
// leaves a half-applied installation. Without apply nothing is created.
func run(client, note string, apply bool, targets []target) (Plan, error) {
	plan := Plan{Client: client, Applied: apply, Note: note, Changes: []Change{}}
	var work []planned
	for _, t := range targets {
		old, exists, err := readTarget(t)
		if err != nil {
			return Plan{}, err
		}
		content, err := t.render(old, exists)
		if err != nil {
			return Plan{}, fmt.Errorf("%s: %w", t.display(), err)
		}
		p := planned{target: t, content: content, action: ActionUnchanged}
		switch {
		case !exists:
			p.action = ActionCreate
		case string(old) != string(content):
			p.action = ActionUpdate
		}
		work = append(work, p)
		plan.Changes = append(plan.Changes, Change{Path: t.display(), Action: p.action, Diff: lineDiff(string(old), string(content))})
	}
	if !apply {
		for _, c := range plan.Changes {
			if c.Action != ActionUnchanged {
				plan.Next = "Review the diffs, then re-run with --yes to apply."
			}
		}
		return plan, nil
	}
	for _, p := range work {
		if p.action == ActionUnchanged {
			continue
		}
		if err := writeTarget(p.target, p.content); err != nil {
			return Plan{}, err
		}
	}
	return plan, nil
}

func readTarget(t target) ([]byte, bool, error) {
	root, err := os.OpenRoot(t.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("open %s: %w", t.dir, err)
	}
	defer root.Close()
	info, err := root.Lstat(t.name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("inspect %s: %w", t.display(), err)
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("refusing to modify %s: not a regular file (symlink?)", t.display())
	}
	b, err := root.ReadFile(t.name)
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", t.display(), err)
	}
	return b, true, nil
}

// writeTarget replaces the file atomically via a temporary file and rename.
func writeTarget(t target, content []byte) error {
	if err := os.MkdirAll(t.dir, 0700); err != nil {
		return fmt.Errorf("create %s: %w", t.dir, err)
	}
	root, err := os.OpenRoot(t.dir)
	if err != nil {
		return fmt.Errorf("open %s: %w", t.dir, err)
	}
	defer root.Close()
	if dir := filepath.Dir(filepath.FromSlash(t.name)); dir != "." {
		if err := root.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	tmp := t.name + ".codememory-tmp"
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, t.mode)
	if err != nil {
		return fmt.Errorf("write %s: %w", t.display(), err)
	}
	_, err = f.Write(content)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = root.Rename(tmp, t.name)
	}
	if err != nil {
		_ = root.Remove(tmp)
		return fmt.Errorf("write %s: %w", t.display(), err)
	}
	return nil
}
