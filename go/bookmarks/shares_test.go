package main

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestShareBookmarkPublic(t *testing.T) {
	s := openTestDB(t)
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	vReq(mux, http.MethodPost, "/bookmarks", `{"url":"https://x","title":"X","description":""}`)
	rec := vReq(mux, http.MethodPost, "/bookmarks/1/share", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create share: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"public_path":"/public/`) {
		t.Fatal(rec.Body.String())
	}
	token := strings.TrimPrefix(strings.Split(strings.Split(rec.Body.String(), `"public_path":"`)[1], `"`)[0], "/public/")

	rec = vReq(mux, http.MethodGet, "/public/"+token, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"kind":"bookmark"`) {
		t.Fatalf("public get: %d %s", rec.Code, rec.Body.String())
	}

	rec = vReq(mux, http.MethodDelete, "/public/"+token, "")
	if rec.Code != http.StatusNoContent {
		t.Fatal(rec.Code)
	}
	rec = vReq(mux, http.MethodGet, "/public/"+token, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("after revoke: %d", rec.Code)
	}
	rec = vReq(mux, http.MethodDelete, "/public/"+token, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("revoke again: %d", rec.Code)
	}
}

func TestShareCollectionOrdered(t *testing.T) {
	s := openTestDB(t)
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	vReq(mux, http.MethodPost, "/collections", `{"name":"C"}`)
	vReq(mux, http.MethodPost, "/bookmarks", `{"url":"https://a","title":"A"}`)
	vReq(mux, http.MethodPost, "/bookmarks", `{"url":"https://b","title":"B"}`)
	vReq(mux, http.MethodPut, "/collections/1/bookmarks/2", "")
	vReq(mux, http.MethodPut, "/collections/1/bookmarks/1", "")

	rec := vReq(mux, http.MethodPost, "/collections/1/share", "")
	if rec.Code != http.StatusCreated {
		t.Fatal(rec.Code)
	}
	token := strings.TrimPrefix(strings.Split(strings.Split(rec.Body.String(), `"public_path":"`)[1], `"`)[0], "/public/")

	rec = vReq(mux, http.MethodGet, "/public/"+token, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"kind":"collection"`) {
		t.Fatal(rec.Body.String())
	}
	if strings.Index(rec.Body.String(), `"title":"B"`) > strings.Index(rec.Body.String(), `"title":"A"`) {
		t.Fatal("expected collection order B then A")
	}
}

func TestShareExpires(t *testing.T) {
	s := openTestDB(t)
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	vReq(mux, http.MethodPost, "/bookmarks", `{"url":"u","title":"T"}`)
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	rec := vReq(mux, http.MethodPost, "/bookmarks/1/share", `{"expires_at":"`+past+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatal(rec.Code)
	}
	token := strings.TrimPrefix(strings.Split(strings.Split(rec.Body.String(), `"token":"`)[1], `"`)[0], "")
	rec = vReq(mux, http.MethodGet, "/public/"+token, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expired share should 404, got %d", rec.Code)
	}
}

func TestShareTTL(t *testing.T) {
	s := openTestDB(t)
	b, _ := s.CreateBookmark("u", "T", "")
	exp := time.Now().UTC().Add(2 * time.Hour)
	sl, err := s.CreateBookmarkShare(b.ID, &exp)
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.ResolvePublicShare(sl.Token, time.Now().UTC())
	if err != nil || view.Bookmark == nil {
		t.Fatal(err)
	}
	_, err = s.ResolvePublicShare(sl.Token, time.Now().UTC().Add(3*time.Hour))
	if err != ErrShareNotFound {
		t.Fatalf("got %v", err)
	}
}
