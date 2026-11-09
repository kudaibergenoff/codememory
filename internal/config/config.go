// Package config owns project-local settings and private state paths.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Config struct {
	Version       int   `json:"version"`
	MaxFileBytes  int64 `json:"max_file_bytes"`
	MaxTotalBytes int64 `json:"max_total_bytes"`
	MaxFiles      int   `json:"max_files"`
	ChunkLines    int   `json:"chunk_lines"`
}

func Default() Config { return Config{1, 1 << 20, 128 << 20, 20000, 40} }
func (c Config) Validate() error {
	if c.Version != 1 || c.MaxFileBytes < 1 || c.MaxFileBytes > 8<<20 || c.MaxTotalBytes < c.MaxFileBytes || c.MaxTotalBytes > 1<<30 || c.MaxFiles < 1 || c.MaxFiles > 100000 || c.ChunkLines < 1 || c.ChunkLines > 200 {
		return errors.New("invalid config version or limits")
	}
	return nil
}

func Root(path string) (string, error) {
	p, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	p, err = filepath.EvalSymlinks(p)
	if err != nil {
		return "", errors.New("project directory does not exist")
	}
	info, err := os.Stat(p)
	if err != nil || !info.IsDir() {
		return "", errors.New("project must be a directory")
	}
	home, _ := os.UserHomeDir()
	if canonical, e := filepath.EvalSymlinks(home); e == nil {
		home = canonical
	}
	if p == filepath.Dir(p) || p == home {
		return "", errors.New("filesystem root and home directory are not project roots")
	}
	return p, nil
}

// State validates the private state directory. It never follows a state symlink.
func State(root string, create bool) (string, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer r.Close()
	if create {
		if err := r.Mkdir(".codememory", 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	st, err := r.Lstat(".codememory")
	if err != nil {
		return "", errors.New("project not initialized; run codememory init")
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("state must be a real directory")
	}
	for _, name := range []string{"config.json", "index.db", "index.db-wal", "index.db-shm", "index.db-journal"} {
		info, e := r.Lstat(filepath.Join(".codememory", name))
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return "", e
		}
		if e == nil && !info.Mode().IsRegular() {
			return "", errors.New("state files must be regular files")
		}
	}
	return filepath.Join(root, ".codememory"), nil
}

func Init(root string) error {
	dir, err := State(root, true)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "config.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		_, err = Load(root)
		return err
	}
	if err != nil {
		return err
	}
	err = json.NewEncoder(f).Encode(Default())
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func Load(root string) (Config, error) {
	dir, err := State(root, false)
	if err != nil {
		return Config{}, err
	}
	f, err := os.Open(filepath.Join(dir, "config.json"))
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil {
		return Config{}, err
	}
	if len(data) > 16384 {
		return Config{}, errors.New("config exceeds 16384 bytes")
	}
	var c Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("read config: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return c, errors.New("unexpected trailing config data")
	}
	return c, c.Validate()
}
