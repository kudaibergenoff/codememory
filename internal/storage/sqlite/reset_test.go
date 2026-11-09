package sqlite

import (
	"context"
	"testing"
)

func TestResetIndexKeepsMemoriesAndEvents(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir(), "p")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, stmt := range []string{
		`INSERT INTO files(id,path,sha256,size_bytes,language) VALUES(1,'a.go','h',1,'go')`,
		`INSERT INTO chunks(file_id,path,symbol,start_line,end_line,content) VALUES(1,'a.go','A',1,1,'alpha')`,
		`INSERT INTO skipped_files(path,size_bytes,mtime_ns,reason) VALUES('b.bin',1,1,'content')`,
		`INSERT INTO query_cache(cache_key,generation,response,created_at_ns,expires_at_ns) VALUES('k',1,'{}',1,9)`,
		`INSERT INTO memories(scope,title,body,sources_json,status,created_at,updated_at) VALUES('*','t','b','[]','unverified','x','x')`,
		`INSERT INTO events(at,at_ns,channel,tool,generation,response_bytes,estimated_tokens,latency_us,failed) VALUES('x',1,'cli','index',1,0,0,0,0)`,
		`UPDATE project SET generation=4,config_hash='c',last_scan_at='x'`,
	} {
		if _, err = s.DB.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	before, err := s.Footprint(ctx)
	if err != nil || before.Files != 1 || before.Memories != 1 || before.CacheEntries != 1 {
		t.Fatalf("footprint %+v %v", before, err)
	}

	if err = s.ResetIndex(ctx); err != nil {
		t.Fatal(err)
	}

	after, err := s.Footprint(ctx)
	if err != nil || after.Files != 0 || after.Chunks != 0 || after.CacheEntries != 0 || after.Memories != 1 || after.Events != 1 {
		t.Fatalf("after reset %+v %v", after, err)
	}
	var fts, generation int
	var cfg string
	s.DB.QueryRow(`SELECT count(*) FROM chunks_fts WHERE chunks_fts MATCH 'alpha'`).Scan(&fts)
	s.DB.QueryRow(`SELECT generation,config_hash FROM project`).Scan(&generation, &cfg)
	if fts != 0 || generation != 4 || cfg != "" {
		t.Fatalf("fts=%d generation=%d config=%q", fts, generation, cfg)
	}
}
