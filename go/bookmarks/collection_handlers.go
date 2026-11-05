package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

type collectionInput struct {
	Name string `json:"name"`
}

type moveBookmarkInput struct {
	Position *int `json:"position"`
}

func parseCollectionInput(r *http.Request) (*collectionInput, error) {
	var in collectionInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return nil, errors.New("body must be a JSON object")
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return nil, errors.New("name is required")
	}
	return &in, nil
}

func parseMoveBookmarkInput(r *http.Request) (int, error) {
	var in moveBookmarkInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return 0, errors.New("body must be a JSON object")
	}
	if in.Position == nil {
		return 0, errors.New("position is required")
	}
	return *in.Position, nil
}

func pathBookmarkID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("bookmark_id"), 10, 64)
	return id, err == nil
}

func storeErr(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, ErrCollectionNotFound):
		writeError(w, http.StatusNotFound, "collection not found")
		return true
	case errors.Is(err, ErrBookmarkNotFound):
		writeError(w, http.StatusNotFound, "bookmark not found")
		return true
	case errors.Is(err, ErrNotInCollection):
		writeError(w, http.StatusNotFound, "bookmark not in collection")
		return true
	case errors.Is(err, ErrPositionOutOfRange):
		writeError(w, http.StatusBadRequest, "position out of range")
		return true
	default:
		return false
	}
}

func (s *Store) listCollections(w http.ResponseWriter, r *http.Request) {
	list, err := s.ListCollections()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Store) createCollection(w http.ResponseWriter, r *http.Request) {
	in, err := parseCollectionInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	c, err := s.CreateCollection(in.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Store) getCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	c, err := s.GetCollection(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if c == nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Store) updateCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	in, err := parseCollectionInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	c, err := s.UpdateCollection(id, in.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if c == nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Store) deleteCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	deleted, err := s.DeleteCollection(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = deleted
	w.WriteHeader(http.StatusNoContent)
}

func (s *Store) addCollectionBookmark(w http.ResponseWriter, r *http.Request) {
	collectionID, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	bookmarkID, ok := pathBookmarkID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "bookmark not found")
		return
	}
	created, err := s.AddCollectionBookmark(collectionID, bookmarkID)
	if err != nil {
		if storeErr(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	b, err := s.GetBookmark(bookmarkID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, b)
}

func (s *Store) removeCollectionBookmark(w http.ResponseWriter, r *http.Request) {
	collectionID, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	bookmarkID, ok := pathBookmarkID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "bookmark not found")
		return
	}
	if err := s.RemoveCollectionBookmark(collectionID, bookmarkID); err != nil {
		if storeErr(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Store) listCollectionBookmarks(w http.ResponseWriter, r *http.Request) {
	collectionID, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	bookmarks, err := s.ListCollectionBookmarks(collectionID)
	if err != nil {
		if storeErr(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, bookmarks)
}

func (s *Store) moveCollectionBookmark(w http.ResponseWriter, r *http.Request) {
	collectionID, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	bookmarkID, ok := pathBookmarkID(r)
	if !ok {
		writeError(w, http.StatusNotFound, "bookmark not found")
		return
	}
	newPos, err := parseMoveBookmarkInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	b, err := s.MoveCollectionBookmark(collectionID, bookmarkID, newPos)
	if err != nil {
		if storeErr(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, b)
}
