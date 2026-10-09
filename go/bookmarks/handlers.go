package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

type bookmarkInput struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func parseBookmarkInput(r *http.Request) (*bookmarkInput, error) {
	var in bookmarkInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return nil, errors.New("body must be a JSON object")
	}
	in.URL = strings.TrimSpace(in.URL)
	in.Title = strings.TrimSpace(in.Title)
	if in.URL == "" {
		return nil, errors.New("url is required")
	}
	if in.Title == "" {
		return nil, errors.New("title is required")
	}
	return &in, nil
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil
}

func (s *Store) listBookmarks(w http.ResponseWriter, r *http.Request) {
	bookmarks, err := s.ListBookmarks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, bookmarks)
}

func (s *Store) getBookmark(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "bookmark not found")
		return
	}
	b, err := s.GetBookmark(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if b == nil {
		writeError(w, http.StatusNotFound, "bookmark not found")
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Store) createBookmark(w http.ResponseWriter, r *http.Request) {
	in, err := parseBookmarkInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	b, err := s.CreateBookmark(in.URL, in.Title, in.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (s *Store) updateBookmark(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "bookmark not found")
		return
	}
	in, err := parseBookmarkInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	b, err := s.UpdateBookmark(id, in.URL, in.Title, in.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if b == nil {
		writeError(w, http.StatusNotFound, "bookmark not found")
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Store) deleteBookmark(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "bookmark not found")
		return
	}
	deleted, err := s.DeleteBookmark(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "bookmark not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
