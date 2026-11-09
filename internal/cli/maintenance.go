package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kudaibergenoff/codememory/internal/benchmark"
	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/doctor"
	"github.com/kudaibergenoff/codememory/internal/install"
	"github.com/kudaibergenoff/codememory/internal/search"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

const applyHint = "Nothing changed. Re-run with --yes to apply."

func writeJSON(out io.Writer, v any) error {
	payload, err := search.Encode(v)
	if err != nil {
		return err
	}
	_, err = out.Write(append(payload, '\n'))
	return err
}

func runInstall(root string, opts options, out io.Writer) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate codememory binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	o := install.Options{Root: root, Executable: exe, Apply: opts.yes}
	var plan install.Plan
	switch opts.client {
	case "claude":
		plan, err = install.Claude(o)
	case "codex":
		plan, err = install.Codex(o)
	default:
		return fmt.Errorf("usage: codememory install claude|codex [--yes]")
	}
	if err != nil {
		return err
	}
	return writeJSON(out, plan)
}

type cleanResult struct {
	Applied bool             `json:"applied"`
	Scope   string           `json:"scope"`
	Before  sqlite.Footprint `json:"before"`
	Kept    []string         `json:"kept"`
	Next    string           `json:"next,omitempty"`
}

// runClean drops derived index data, or with --all the whole state directory
// (including memories and analytics). Without --yes it only reports.
func runClean(ctx context.Context, root string, opts options, out io.Writer) error {
	dir, err := config.State(root, false)
	if err != nil {
		return err
	}
	s, err := sqlite.Open(ctx, dir, root)
	if err != nil {
		return err
	}
	before, err := s.Footprint(ctx)
	if err != nil {
		s.Close()
		return err
	}
	res := cleanResult{Applied: opts.yes, Scope: "index", Before: before, Kept: []string{"memories", "analytics", "config"}}
	if opts.all {
		res.Scope, res.Kept = "all", []string{}
	}
	switch {
	case !opts.yes:
		res.Next = applyHint
		err = s.Close()
	case opts.all:
		if err = s.Close(); err == nil {
			err = os.RemoveAll(dir) // dir is the validated, non-symlink .codememory
		}
	default:
		err = s.ResetIndex(ctx)
		if closeErr := s.Close(); err == nil {
			err = closeErr
		}
	}
	if err != nil {
		return err
	}
	return writeJSON(out, res)
}

func runDoctor(ctx context.Context, root string, out io.Writer) error {
	return writeJSON(out, doctor.Run(ctx, root, Version))
}

// runBenchmark runs the built-in suite (temporary fixture copy) or a task file
// against root, and keeps the report for optimize and dashboard.
func runBenchmark(ctx context.Context, s *sqlite.Store, root string, cfg config.Config, tasks string) (any, error) {
	var report benchmark.Report
	var err error
	if tasks == "" {
		report, err = benchmark.Builtin(ctx)
	} else {
		f, openErr := os.Open(tasks)
		if openErr != nil {
			return nil, fmt.Errorf("open tasks: %w", openErr)
		}
		defer f.Close()
		suite, loadErr := benchmark.LoadSuite(f)
		if loadErr != nil {
			return nil, loadErr
		}
		report, err = benchmark.RunSuite(ctx, s, root, cfg, suite, filepath.Base(tasks))
	}
	if err != nil {
		return nil, err
	}
	payload, err := search.Encode(report)
	if err != nil {
		return nil, err
	}
	if err := s.SaveBenchmark(ctx, report.Suite, payload); err != nil {
		return nil, err
	}
	return report, nil
}
