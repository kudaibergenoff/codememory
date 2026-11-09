package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// seedVersion1 creates a database exactly as the first release left it.
func seedVersion1(t *testing.T, dir, root string) {
	t.Helper()
	db, err := sql.Open(driverName, dsn(filepath.Join(dir, "index.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	initial, err := migrations.ReadFile("migrations/001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		string(initial),
		"INSERT INTO project(id,root,generation,last_scan_at) VALUES(1,'" + root + "',3,'2026-11-09T00:00:00Z')",
		"INSERT INTO files(id,path,sha256,size_bytes,language) VALUES(1,'a.go','hash',10,'go')",
		"PRAGMA user_version=1",
	} {
		if _, err = db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUpgradeFromVersion1KeepsDataAndForcesStatRecheck(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	seedVersion1(t, dir, "project-a")

	s, err := Open(ctx, dir, "project-a")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var version int
	if err = s.DB.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != schemaVersion {
		t.Fatalf("user_version=%d %v", version, err)
	}
	prior, configHash, err := s.Prior(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := prior["a.go"]; got.Hash != "hash" || got.Size != 10 || got.MtimeNS != 0 {
		t.Fatalf("migrated entry %+v", got)
	}
	if configHash != "" {
		t.Fatalf("config hash %q", configHash)
	}
	if st, err := s.Status(ctx); err != nil || st.Generation != 3 {
		t.Fatalf("generation lost: %+v %v", st, err)
	}
}

func TestNewerSchemaIsRejected(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(ctx, dir, "project-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("PRAGMA user_version=99"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	if s, err = Open(ctx, dir, "project-a"); err == nil {
		s.Close()
		t.Fatal("opened a database from a newer binary")
	}
}
