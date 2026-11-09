package scanner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kudaibergenoff/codememory/internal/config"
)

const (
	maxDepth       = 64
	maxEntries     = 200000
	maxIgnoreBytes = 64 << 10
	maxIgnoreRules = 4096
	readDirBatch   = 128
	// racyWindow: a file modified this close to (or after) the scan start may
	// change again within the filesystem's timestamp granularity without its
	// mtime moving, so its stat is recorded as unsettled and re-read next time.
	racyWindow = 2 * time.Second
)

var ignoreFiles = [...]string{".gitignore", ".codememoryignore"}

type walker struct {
	ctx      context.Context
	root     *os.Root
	cfg      config.Config
	prior    Prior
	started  time.Time
	snap     Snapshot
	total    int64
	entries  int
	patterns int
}

func (w *walker) walk(dir string, gitRules, extraRules []rule, depth int) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if depth > maxDepth {
		return errors.New("directory depth limit exceeded")
	}
	gitRules, extraRules, err := w.loadIgnores(dir, gitRules, extraRules)
	if err != nil {
		return err
	}
	d, err := w.root.Open(dir)
	if err != nil {
		return errors.New("cannot open directory")
	}
	defer d.Close()
	for {
		batch, readErr := d.ReadDir(readDirBatch)
		if readErr != nil && readErr != io.EOF {
			return errors.New("cannot list directory")
		}
		for _, entry := range batch {
			if err := w.visit(dir, entry, gitRules, extraRules, depth); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			return nil
		}
	}
}

// loadIgnores returns copies of the inherited rules extended with this
// directory's ignore files; sibling directories never share appended rules.
func (w *walker) loadIgnores(dir string, gitRules, extraRules []rule) ([]rule, []rule, error) {
	sets := [len(ignoreFiles)][]rule{append([]rule(nil), gitRules...), append([]rule(nil), extraRules...)}
	base := dir
	if base == "." {
		base = ""
	}
	for i, name := range ignoreFiles {
		rules, err := w.ignoreFile(path.Join(dir, name), base)
		if err != nil {
			return nil, nil, err
		}
		w.patterns += len(rules)
		if w.patterns > maxIgnoreRules {
			return nil, nil, errors.New("ignore rule limit exceeded")
		}
		sets[i] = append(sets[i], rules...)
	}
	return sets[0], sets[1], nil
}

func (w *walker) ignoreFile(rel, base string) ([]rule, error) {
	info, err := w.root.Lstat(rel)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("cannot inspect ignore file")
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("ignore files must be regular, not symlinks")
	}
	b, err := read(w.root, rel, maxIgnoreBytes)
	if err != nil {
		return nil, errors.New("cannot read bounded ignore file")
	}
	if !utf8.Valid(b) {
		return nil, errors.New("ignore file is not UTF-8")
	}
	return parseRules(base, string(b))
}

func (w *walker) visit(dir string, entry fs.DirEntry, gitRules, extraRules []rule, depth int) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	w.entries++
	if w.entries > maxEntries {
		return errors.New("directory entry limit exceeded")
	}
	rel := path.Join(dir, entry.Name())
	isDir := entry.IsDir()
	if Denied(entry.Name()) || entry.Type()&os.ModeSymlink != 0 || ignored(gitRules, rel, isDir) || ignored(extraRules, rel, isDir) {
		w.snap.Skipped++
		return nil
	}
	if isDir {
		return w.walk(rel, gitRules, extraRules, depth+1)
	}
	return w.file(rel)
}

func (w *walker) file(rel string) error {
	info, err := w.root.Lstat(rel)
	if errors.Is(err, os.ErrNotExist) {
		return nil // removed after listing (e.g. editor temp file): treated as deleted
	}
	if err != nil {
		return errors.New("cannot inspect file")
	}
	if !info.Mode().IsRegular() || info.Size() > w.cfg.MaxFileBytes {
		w.snap.Skipped++
		return nil
	}
	// The stat is taken before reading: a write racing with the read leaves a
	// stat that no longer matches, so the next scan re-reads the file.
	size, mtime := info.Size(), w.settled(info.ModTime())
	if p, ok := w.prior[rel]; ok && p.matches(size, mtime) {
		if p.Skip != "" {
			w.skip(Skipped{Path: rel, Size: size, MtimeNS: mtime, Reason: p.Skip})
			return nil
		}
		return w.add(File{Path: rel, Hash: p.Hash, Language: Language(rel), Size: size, MtimeNS: mtime, Reused: true})
	}
	b, err := read(w.root, rel, w.cfg.MaxFileBytes)
	if err != nil {
		return errors.New("file changed or could not be safely read; retry")
	}
	if reason := classify(b); reason != "" {
		w.skip(Skipped{Path: rel, Size: size, MtimeNS: mtime, Reason: reason})
		return nil
	}
	return w.add(File{Path: rel, Hash: Hash(b), Language: Language(rel), Content: b, Size: size, MtimeNS: mtime})
}

// settled returns the mtime in nanoseconds, or 0 when it is too recent to trust.
func (w *walker) settled(mtime time.Time) int64 {
	if !mtime.Before(w.started.Add(-racyWindow)) {
		return 0
	}
	return mtime.UnixNano()
}

func (w *walker) skip(s Skipped) {
	w.snap.Skipped++
	if s.Reason == SkipSecret {
		w.snap.Secrets++
	}
	w.snap.Skips = append(w.snap.Skips, s)
}

func (w *walker) add(f File) error {
	w.total += f.Size
	if w.total > w.cfg.MaxTotalBytes || len(w.snap.Files) >= w.cfg.MaxFiles {
		return errors.New("project size limit exceeded; adjust config or ignore rules")
	}
	w.snap.Files = append(w.snap.Files, f)
	return nil
}

func classify(b []byte) SkipReason {
	isControl := func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }
	if bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b) || bytes.IndexFunc(b, isControl) >= 0 {
		return SkipContent
	}
	if secret.Match(b) {
		return SkipSecret
	}
	return ""
}
