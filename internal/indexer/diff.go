package indexer

import (
	"context"

	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/parser"
	"github.com/kudaibergenoff/codememory/internal/scanner"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

// delta is the difference between the recorded index and one project scan.
type delta struct {
	snap         scanner.Snapshot
	changed      []scanner.File // new or different content; Content is set
	deleted      []string
	touched      []sqlite.Stat // same content, different recorded stat
	reused       int
	skipsChanged bool
}

// diff scans root and classifies every file against prior. When full is true
// (the configuration changed) the stat prefilter is bypassed and every file
// is read and treated as changed.
func diff(ctx context.Context, root string, cfg config.Config, prior scanner.Prior, full bool) (delta, error) {
	scanPrior := prior
	if full {
		scanPrior = nil
	}
	snap, err := scanner.Scan(ctx, root, cfg, scanPrior)
	if err != nil {
		return delta{}, err
	}
	d := delta{snap: snap}
	seen := make(map[string]bool, len(snap.Files))
	for i, f := range snap.Files {
		seen[f.Path] = true
		d.classify(f, prior, full)
		snap.Files[i].Content = nil // only changed files keep their bytes
	}
	for p, e := range prior {
		if e.Skip == "" && !seen[p] {
			d.deleted = append(d.deleted, p)
		}
	}
	d.skipsChanged = !sameSkips(prior, snap.Skips)
	return d, nil
}

func (d *delta) classify(f scanner.File, prior scanner.Prior, full bool) {
	old, ok := prior[f.Path]
	if full || !ok || old.Skip != "" || old.Hash != f.Hash {
		d.changed = append(d.changed, f)
		return
	}
	d.reused++
	if old.Size != f.Size || old.MtimeNS != f.MtimeNS {
		d.touched = append(d.touched, sqlite.Stat{Path: f.Path, Size: f.Size, MtimeNS: f.MtimeNS})
	}
}

func sameSkips(prior scanner.Prior, skips []scanner.Skipped) bool {
	recorded := 0
	for _, e := range prior {
		if e.Skip != "" {
			recorded++
		}
	}
	if recorded != len(skips) {
		return false
	}
	for _, s := range skips {
		e, ok := prior[s.Path]
		if !ok || e.Skip != s.Reason || e.Size != s.Size || e.MtimeNS != s.MtimeNS {
			return false
		}
	}
	return true
}

// batch parses changed files and packages the delta for storage.
func (d delta) batch(cfg config.Config, cfgHash string) sqlite.Batch {
	b := sqlite.Batch{
		ConfigHash: cfgHash, Deleted: d.deleted, Touched: d.touched,
		Skips: d.snap.Skips, SkipsChanged: d.skipsChanged,
		Reused: d.reused, Skipped: d.snap.Skipped, Secrets: d.snap.Secrets,
	}
	for i, f := range d.changed {
		parsed := parser.Parse(f.Path, f.Language, f.Content, cfg.ChunkLines)
		d.changed[i].Content = nil // the parser copied it; free the bytes early
		if parsed.ParseError {
			b.ParseErrors++
		}
		b.Changed = append(b.Changed, sqlite.Document{
			Path: f.Path, Hash: f.Hash, Language: f.Language,
			Size: f.Size, MtimeNS: f.MtimeNS, Parsed: parsed,
		})
	}
	return b
}
