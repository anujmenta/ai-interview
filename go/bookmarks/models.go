package main

import "database/sql"

type Bookmark struct {
	ID          int64  `json:"id"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
}

type Store struct {
	db *sql.DB
}

const bookmarkCols = "id, url, title, description, created_at"

func scanBookmark(row interface{ Scan(...any) error }) (*Bookmark, error) {
	var b Bookmark
	if err := row.Scan(&b.ID, &b.URL, &b.Title, &b.Description, &b.CreatedAt); err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Store) ListBookmarks() ([]Bookmark, error) {
	rows, err := s.db.Query("SELECT " + bookmarkCols + " FROM bookmarks ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bookmarks := []Bookmark{}
	for rows.Next() {
		b, err := scanBookmark(rows)
		if err != nil {
			return nil, err
		}
		bookmarks = append(bookmarks, *b)
	}
	return bookmarks, rows.Err()
}

// GetBookmark returns (nil, nil) when the bookmark does not exist.
func (s *Store) GetBookmark(id int64) (*Bookmark, error) {
	b, err := scanBookmark(s.db.QueryRow("SELECT "+bookmarkCols+" FROM bookmarks WHERE id = ?", id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return b, err
}

func (s *Store) CreateBookmark(url, title, description string) (*Bookmark, error) {
	res, err := s.db.Exec(
		"INSERT INTO bookmarks (url, title, description) VALUES (?, ?, ?)",
		url, title, description,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetBookmark(id)
}

// UpdateBookmark returns (nil, nil) when the bookmark does not exist.
func (s *Store) UpdateBookmark(id int64, url, title, description string) (*Bookmark, error) {
	res, err := s.db.Exec(
		"UPDATE bookmarks SET url = ?, title = ?, description = ? WHERE id = ?",
		url, title, description, id,
	)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, nil
	}
	return s.GetBookmark(id)
}

func (s *Store) DeleteBookmark(id int64) (bool, error) {
	res, err := s.db.Exec("DELETE FROM bookmarks WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
