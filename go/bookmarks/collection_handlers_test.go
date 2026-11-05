package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testServer(t *testing.T) *Store {
	t.Helper()
	s := openTestDB(t)
	return s
}

func TestHTTPCollections(t *testing.T) {
	store := testServer(t)
	mux := http.NewServeMux()
	store.registerRoutes(mux)

	t.Run("collection CRUD", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/collections", strings.NewReader(`{"name":"List"}`))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
		}

		req = httptest.NewRequest(http.MethodGet, "/collections", nil)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "List") {
			t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
		}

		req = httptest.NewRequest(http.MethodPost, "/collections", strings.NewReader(`{"name":""}`))
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("bad name: %d", rec.Code)
		}
	})

	t.Run("membership idempotent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/bookmarks", strings.NewReader(`{"url":"https://x","title":"X"}`))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatal(rec.Body.String())
		}
		req = httptest.NewRequest(http.MethodPost, "/collections", strings.NewReader(`{"name":"C"}`))
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		req = httptest.NewRequest(http.MethodPut, "/collections/1/bookmarks/1", nil)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("first add: %d", rec.Code)
		}
		req = httptest.NewRequest(http.MethodPut, "/collections/1/bookmarks/1", nil)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("second add: %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodDelete, "/collections/1/bookmarks/1", nil)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatal(rec.Code)
		}
		req = httptest.NewRequest(http.MethodDelete, "/collections/1/bookmarks/1", nil)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatal(rec.Code)
		}
	})

	t.Run("move validation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/collections/1/bookmarks/1/position", strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("missing position: %d", rec.Code)
		}
	})

	t.Run("bookmarks regression", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/bookmarks", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatal(rec.Code)
		}
	})
}
