package main

import (
	"database/sql"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS bookmarks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS collections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS collection_bookmarks (
    collection_id INTEGER NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    bookmark_id INTEGER NOT NULL REFERENCES bookmarks(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    PRIMARY KEY (collection_id, bookmark_id),
    UNIQUE (collection_id, position)
);

CREATE TABLE IF NOT EXISTS share_links (
    token TEXT PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('bookmark', 'collection')),
    bookmark_id INTEGER REFERENCES bookmarks(id) ON DELETE CASCADE,
    collection_id INTEGER REFERENCES collections(id) ON DELETE CASCADE,
    revoked_at TEXT,
    expires_at TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (
        (kind = 'bookmark' AND bookmark_id IS NOT NULL AND collection_id IS NULL) OR
        (kind = 'collection' AND collection_id IS NOT NULL AND bookmark_id IS NULL)
    )
);`

func openDB() (*sql.DB, error) {
	path := os.Getenv("DB_PATH")
	if path == "" {
		path = "bookmarks.db"
	}
	dsn := path
	if strings.Contains(path, "?") {
		dsn = path + "&_pragma=foreign_keys(1)"
	} else {
		dsn = path + "?_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	return db, nil
}
