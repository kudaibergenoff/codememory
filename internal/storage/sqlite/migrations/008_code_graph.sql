-- Code graph (Go only): one row per distinct reference from a top-level
-- function or method to a name, from go/ast without type checking. Rows are
-- filled when files are (re)parsed; parser v3 forces a full re-parse.
CREATE TABLE refs (
 id INTEGER PRIMARY KEY,
 file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
 dir TEXT NOT NULL,       -- directory of the referencing file, '.' at the root
 from_name TEXT NOT NULL, -- Func or Recv.Method
 to_pkg TEXT NOT NULL,    -- import path; '' same package; '.' method on a value
 to_name TEXT NOT NULL,
 kind TEXT NOT NULL,      -- call | ref | method
 line INTEGER NOT NULL,   -- first use
 uses INTEGER NOT NULL
);
CREATE INDEX refs_target ON refs(to_name);
CREATE INDEX refs_file ON refs(file_id);
CREATE INDEX refs_from ON refs(dir, from_name);
ALTER TABLE files ADD COLUMN dir TEXT NOT NULL DEFAULT '';
CREATE INDEX files_dir ON files(dir);
CREATE INDEX symbols_name ON symbols(name);
