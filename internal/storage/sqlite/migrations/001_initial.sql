CREATE TABLE project (
 id INTEGER PRIMARY KEY CHECK(id=1), root TEXT NOT NULL,
 generation INTEGER NOT NULL DEFAULT 0, last_scan_at TEXT NOT NULL DEFAULT '',
 config_hash TEXT NOT NULL DEFAULT ''
);
CREATE TABLE files (
 id INTEGER PRIMARY KEY, path TEXT NOT NULL UNIQUE, sha256 TEXT NOT NULL,
 size_bytes INTEGER NOT NULL, language TEXT NOT NULL
);
CREATE TABLE symbols (
 id INTEGER PRIMARY KEY, file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
 name TEXT NOT NULL, kind TEXT NOT NULL, start_line INTEGER NOT NULL, end_line INTEGER NOT NULL
);
CREATE INDEX symbols_file ON symbols(file_id);
CREATE TABLE chunks (
 id INTEGER PRIMARY KEY, file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
 path TEXT NOT NULL, symbol TEXT NOT NULL, start_line INTEGER NOT NULL,
 end_line INTEGER NOT NULL, content TEXT NOT NULL
);
CREATE INDEX chunks_file ON chunks(file_id);
CREATE VIRTUAL TABLE chunks_fts USING fts5(path, symbol, content, content='chunks', content_rowid='id', tokenize='unicode61');
CREATE TRIGGER chunks_ai AFTER INSERT ON chunks BEGIN
 INSERT INTO chunks_fts(rowid,path,symbol,content) VALUES(new.id,new.path,new.symbol,new.content);
END;
CREATE TRIGGER chunks_ad AFTER DELETE ON chunks BEGIN
 INSERT INTO chunks_fts(chunks_fts,rowid,path,symbol,content) VALUES('delete',old.id,old.path,old.symbol,old.content);
END;
CREATE TABLE events (
 id INTEGER PRIMARY KEY, at TEXT NOT NULL, channel TEXT NOT NULL, tool TEXT NOT NULL,
 generation INTEGER NOT NULL, response_bytes INTEGER NOT NULL, estimated_tokens INTEGER NOT NULL,
 latency_us INTEGER NOT NULL, failed INTEGER NOT NULL,
 indexed_files INTEGER NOT NULL DEFAULT 0, reused_files INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX events_at ON events(at);
