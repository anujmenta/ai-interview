package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

type shareInput struct {
	ExpiresAt  *string `json:"expires_at"`
	TTLSeconds *int    `json:"ttl_seconds"`
}

func parseShareInput(r *http.Request) (*time.Time, error) {
	var in shareInput
	err := json.NewDecoder(r.Body).Decode(&in)
	if err == io.EOF {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("body must be a JSON object")
	}
	if in.TTLSeconds != nil {
		if *in.TTLSeconds <= 0 {
			return nil, errors.New("ttl_seconds must be positive")
		}
		t := time.Now().UTC().Add(time.Duration(*in.TTLSeconds) * time.Second)
		return &t, nil
	}
	if in.ExpiresAt != nil {
		if *in.ExpiresAt == "" {
			return nil, errors.New("expires_at must be a valid RFC3339 timestamp")
		}
		t, err := time.Parse(time.RFC3339, *in.ExpiresAt)
		if err != nil {
			return nil, errors.New("expires_at must be a valid RFC3339 timestamp")
		}
		utc := t.UTC()
		return &utc, nil
	}
	return nil, nil
}

func pathToken(r *http.Request) (string, bool) {
	t := r.PathValue("token")
	return t, t != ""
}

func shareErr(w http.ResponseWriter, err error) bool {
	if errors.Is(err, ErrShareNotFound) {
		writeError(w, http.StatusNotFound, "share not found")
		return true
	}
	if errors.Is(err, ErrBookmarkNotFound) {
		writeError(w, http.StatusNotFound, "bookmark not found")
		return true
	}
	if errors.Is(err, ErrCollectionNotFound) {
		writeError(w, http.StatusNotFound, "collection not found")
		return true
	}
	return false
}

func (s *Store) createBookmarkShare(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "bookmark not found")
		return
	}
	expiresAt, err := parseShareInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sl, err := s.CreateBookmarkShare(id, expiresAt)
	if err != nil {
		if shareErr(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, sl)
}

func (s *Store) createCollectionShare(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	expiresAt, err := parseShareInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sl, err := s.CreateCollectionShare(id, expiresAt)
	if err != nil {
		if shareErr(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, sl)
}

func (s *Store) getPublicShare(w http.ResponseWriter, r *http.Request) {
	token, ok := pathToken(r)
	if !ok {
		writeError(w, http.StatusNotFound, "share not found")
		return
	}
	view, err := s.ResolvePublicShare(token, time.Now().UTC())
	if err != nil {
		if shareErr(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Store) revokePublicShare(w http.ResponseWriter, r *http.Request) {
	token, ok := pathToken(r)
	if !ok {
		writeError(w, http.StatusNotFound, "share not found")
		return
	}
	if err := s.RevokeShare(token); err != nil {
		if shareErr(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
