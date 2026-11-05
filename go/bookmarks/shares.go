package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"
)

type ShareLink struct {
	Token        string  `json:"token"`
	Kind         string  `json:"kind"`
	BookmarkID   *int64  `json:"bookmark_id,omitempty"`
	CollectionID *int64  `json:"collection_id,omitempty"`
	ExpiresAt    *string `json:"expires_at,omitempty"`
	RevokedAt    *string `json:"revoked_at,omitempty"`
	CreatedAt    string  `json:"created_at"`
	PublicPath   string  `json:"public_path"`
}

type PublicShareView struct {
	Kind       string      `json:"kind"`
	Bookmark   *Bookmark   `json:"bookmark,omitempty"`
	Collection *Collection `json:"collection,omitempty"`
	Bookmarks  []Bookmark  `json:"bookmarks"`
}

var (
	ErrShareNotFound = errors.New("share not found")
)

func newShareToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func scanShareLink(row interface{ Scan(...any) error }) (*ShareLink, error) {
	var sl ShareLink
	var bookmarkID, collectionID sql.NullInt64
	var expiresAt, revokedAt sql.NullString
	if err := row.Scan(&sl.Token, &sl.Kind, &bookmarkID, &collectionID, &revokedAt, &expiresAt, &sl.CreatedAt); err != nil {
		return nil, err
	}
	if bookmarkID.Valid {
		v := bookmarkID.Int64
		sl.BookmarkID = &v
	}
	if collectionID.Valid {
		v := collectionID.Int64
		sl.CollectionID = &v
	}
	if expiresAt.Valid {
		v := expiresAt.String
		sl.ExpiresAt = &v
	}
	if revokedAt.Valid {
		v := revokedAt.String
		sl.RevokedAt = &v
	}
	sl.PublicPath = "/public/" + sl.Token
	return &sl, nil
}

func shareActive(sl *ShareLink, now time.Time) bool {
	if sl.RevokedAt != nil && *sl.RevokedAt != "" {
		return false
	}
	if sl.ExpiresAt != nil && *sl.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *sl.ExpiresAt)
		if err == nil && !now.Before(t) {
			return false
		}
	}
	return true
}

func (s *Store) GetShareLink(token string) (*ShareLink, error) {
	sl, err := scanShareLink(s.db.QueryRow(
		`SELECT token, kind, bookmark_id, collection_id, revoked_at, expires_at, created_at
		 FROM share_links WHERE token = ?`, token,
	))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return sl, err
}

func (s *Store) CreateBookmarkShare(bookmarkID int64, expiresAt *time.Time) (*ShareLink, error) {
	if b, err := s.GetBookmark(bookmarkID); err != nil {
		return nil, err
	} else if b == nil {
		return nil, ErrBookmarkNotFound
	}
	return s.insertShare("bookmark", &bookmarkID, nil, expiresAt)
}

func (s *Store) CreateCollectionShare(collectionID int64, expiresAt *time.Time) (*ShareLink, error) {
	if c, err := s.GetCollection(collectionID); err != nil {
		return nil, err
	} else if c == nil {
		return nil, ErrCollectionNotFound
	}
	return s.insertShare("collection", nil, &collectionID, expiresAt)
}

func (s *Store) insertShare(kind string, bookmarkID, collectionID *int64, expiresAt *time.Time) (*ShareLink, error) {
	token, err := newShareToken()
	if err != nil {
		return nil, err
	}
	var exp any
	if expiresAt != nil {
		exp = expiresAt.UTC().Format(time.RFC3339)
	}
	_, err = s.db.Exec(
		`INSERT INTO share_links (token, kind, bookmark_id, collection_id, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		token, kind, bookmarkID, collectionID, exp,
	)
	if err != nil {
		return nil, err
	}
	return s.GetShareLink(token)
}

// RevokeShare marks a link revoked. Idempotent for unknown or already-revoked tokens.
func (s *Store) RevokeShare(token string) error {
	_, err := s.db.Exec(
		`UPDATE share_links SET revoked_at = COALESCE(revoked_at, CURRENT_TIMESTAMP) WHERE token = ?`,
		token,
	)
	return err
}

// ResolvePublicShare returns read-only payload for an active share.
func (s *Store) ResolvePublicShare(token string, now time.Time) (*PublicShareView, error) {
	sl, err := s.GetShareLink(token)
	if err != nil {
		return nil, err
	}
	if sl == nil || !shareActive(sl, now) {
		return nil, ErrShareNotFound
	}
	switch sl.Kind {
	case "bookmark":
		if sl.BookmarkID == nil {
			return nil, ErrShareNotFound
		}
		b, err := s.GetBookmark(*sl.BookmarkID)
		if err != nil {
			return nil, err
		}
		if b == nil {
			return nil, ErrShareNotFound
		}
		return &PublicShareView{Kind: "bookmark", Bookmark: b}, nil
	case "collection":
		if sl.CollectionID == nil {
			return nil, ErrShareNotFound
		}
		c, err := s.GetCollection(*sl.CollectionID)
		if err != nil {
			return nil, err
		}
		if c == nil {
			return nil, ErrShareNotFound
		}
		bookmarks, err := s.ListCollectionBookmarks(*sl.CollectionID)
		if err != nil {
			return nil, err
		}
		return &PublicShareView{Kind: "collection", Collection: c, Bookmarks: bookmarks}, nil
	default:
		return nil, ErrShareNotFound
	}
}
