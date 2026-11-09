package sqlite

import (
	"context"
	"testing"
)

func TestMigrationsFTSAndIsolation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(ctx, dir, "project-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO files(id,path,sha256,size_bytes,language) VALUES(1,'a.go','hash',1,'go'); INSERT INTO chunks(file_id,path,symbol,start_line,end_line,content) VALUES(1,'a.go','Pay',1,2,'payment')`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = s.DB.QueryRow(`SELECT count(*) FROM chunks_fts WHERE chunks_fts MATCH 'payment'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("FTS %d %v", n, err)
	}
	if _, err = s.DB.Exec(`DELETE FROM files`); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRow(`SELECT count(*) FROM chunks_fts WHERE chunks_fts MATCH 'payment'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("delete FTS %d %v", n, err)
	}
	s.Close()
	s, err = Open(ctx, dir, "project-a")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if other, err := Open(ctx, dir, "project-b"); err == nil {
		other.Close()
		t.Fatal("cross-project reuse")
	}
}
