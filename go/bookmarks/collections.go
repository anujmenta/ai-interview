package main

import (
	"database/sql"
	"errors"
	"fmt"
)

type Collection struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

var (
	ErrCollectionNotFound = errors.New("collection not found")
	ErrBookmarkNotFound   = errors.New("bookmark not found")
	ErrNotInCollection    = errors.New("bookmark not in collection")
	ErrPositionOutOfRange = errors.New("position out of range")
)

const collectionCols = "id, name, created_at"

func scanCollection(row interface{ Scan(...any) error }) (*Collection, error) {
	var c Collection
	if err := row.Scan(&c.ID, &c.Name, &c.CreatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) ListCollections() ([]Collection, error) {
	rows, err := s.db.Query("SELECT " + collectionCols + " FROM collections ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Collection{}
	for rows.Next() {
		c, err := scanCollection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// GetCollection returns (nil, nil) when the collection does not exist.
func (s *Store) GetCollection(id int64) (*Collection, error) {
	c, err := scanCollection(s.db.QueryRow("SELECT "+collectionCols+" FROM collections WHERE id = ?", id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

func (s *Store) CreateCollection(name string) (*Collection, error) {
	res, err := s.db.Exec("INSERT INTO collections (name) VALUES (?)", name)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetCollection(id)
}

// UpdateCollection returns (nil, nil) when the collection does not exist.
func (s *Store) UpdateCollection(id int64, name string) (*Collection, error) {
	res, err := s.db.Exec("UPDATE collections SET name = ? WHERE id = ?", name, id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, nil
	}
	return s.GetCollection(id)
}

func (s *Store) DeleteCollection(id int64) (bool, error) {
	res, err := s.db.Exec("DELETE FROM collections WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) collectionBookmarkIDsTx(tx *sql.Tx, collectionID int64) ([]int64, error) {
	rows, err := tx.Query(
		"SELECT bookmark_id FROM collection_bookmarks WHERE collection_id = ? ORDER BY position ASC",
		collectionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func writeCollectionPositionsTx(tx *sql.Tx, collectionID int64, bookmarkIDs []int64) error {
	// Two-phase update avoids UNIQUE(collection_id, position) conflicts while reindexing.
	offset := len(bookmarkIDs) + 1
	for i, bid := range bookmarkIDs {
		if _, err := tx.Exec(
			"UPDATE collection_bookmarks SET position = ? WHERE collection_id = ? AND bookmark_id = ?",
			offset+i, collectionID, bid,
		); err != nil {
			return err
		}
	}
	for i, bid := range bookmarkIDs {
		res, err := tx.Exec(
			"UPDATE collection_bookmarks SET position = ? WHERE collection_id = ? AND bookmark_id = ?",
			i, collectionID, bid,
		)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return fmt.Errorf("bookmark %d not in collection %d", bid, collectionID)
		}
	}
	return nil
}

// AddCollectionBookmark adds bookmark to collection. created is false when already a member.
func (s *Store) AddCollectionBookmark(collectionID, bookmarkID int64) (created bool, err error) {
	if c, err := s.GetCollection(collectionID); err != nil {
		return false, err
	} else if c == nil {
		return false, ErrCollectionNotFound
	}
	if b, err := s.GetBookmark(bookmarkID); err != nil {
		return false, err
	} else if b == nil {
		return false, ErrBookmarkNotFound
	}

	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var existing int
	err = tx.QueryRow(
		"SELECT 1 FROM collection_bookmarks WHERE collection_id = ? AND bookmark_id = ?",
		collectionID, bookmarkID,
	).Scan(&existing)
	if err == nil {
		return false, tx.Commit()
	}
	if err != sql.ErrNoRows {
		return false, err
	}

	var maxPos sql.NullInt64
	if err := tx.QueryRow(
		"SELECT MAX(position) FROM collection_bookmarks WHERE collection_id = ?",
		collectionID,
	).Scan(&maxPos); err != nil {
		return false, err
	}
	nextPos := 0
	if maxPos.Valid {
		nextPos = int(maxPos.Int64) + 1
	}

	if _, err := tx.Exec(
		"INSERT INTO collection_bookmarks (collection_id, bookmark_id, position) VALUES (?, ?, ?)",
		collectionID, bookmarkID, nextPos,
	); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveCollectionBookmark removes a bookmark from a collection. Idempotent when not a member.
func (s *Store) RemoveCollectionBookmark(collectionID, bookmarkID int64) error {
	if c, err := s.GetCollection(collectionID); err != nil {
		return err
	} else if c == nil {
		return ErrCollectionNotFound
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var pos int
	err = tx.QueryRow(
		"SELECT position FROM collection_bookmarks WHERE collection_id = ? AND bookmark_id = ?",
		collectionID, bookmarkID,
	).Scan(&pos)
	if err == sql.ErrNoRows {
		return tx.Commit()
	}
	if err != nil {
		return err
	}

	if _, err := tx.Exec(
		"DELETE FROM collection_bookmarks WHERE collection_id = ? AND bookmark_id = ?",
		collectionID, bookmarkID,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		"UPDATE collection_bookmarks SET position = position - 1 WHERE collection_id = ? AND position > ?",
		collectionID, pos,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// ListCollectionBookmarks returns bookmarks in user order. ErrCollectionNotFound if missing.
func (s *Store) ListCollectionBookmarks(collectionID int64) ([]Bookmark, error) {
	if c, err := s.GetCollection(collectionID); err != nil {
		return nil, err
	} else if c == nil {
		return nil, ErrCollectionNotFound
	}

	q := `SELECT b.id, b.url, b.title, b.description, b.created_at
FROM bookmarks b
INNER JOIN collection_bookmarks cb ON cb.bookmark_id = b.id
WHERE cb.collection_id = ?
ORDER BY cb.position ASC`
	rows, err := s.db.Query(q, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Bookmark{}
	for rows.Next() {
		b, err := scanBookmark(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// MoveCollectionBookmark reorders a bookmark within a collection.
func (s *Store) MoveCollectionBookmark(collectionID, bookmarkID int64, newPos int) (*Bookmark, error) {
	if c, err := s.GetCollection(collectionID); err != nil {
		return nil, err
	} else if c == nil {
		return nil, ErrCollectionNotFound
	}
	if b, err := s.GetBookmark(bookmarkID); err != nil {
		return nil, err
	} else if b == nil {
		return nil, ErrBookmarkNotFound
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	ids, err := s.collectionBookmarkIDsTx(tx, collectionID)
	if err != nil {
		return nil, err
	}
	n := len(ids)

	cur := -1
	for i, id := range ids {
		if id == bookmarkID {
			cur = i
			break
		}
	}
	if cur < 0 {
		return nil, ErrNotInCollection
	}
	if newPos < 0 || newPos >= n {
		return nil, ErrPositionOutOfRange
	}
	if cur == newPos {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return s.GetBookmark(bookmarkID)
	}

	moved := ids[cur]
	ids = append(ids[:cur], ids[cur+1:]...)
	ids = append(ids[:newPos], append([]int64{moved}, ids[newPos:]...)...)

	if err := writeCollectionPositionsTx(tx, collectionID, ids); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetBookmark(bookmarkID)
}
