package main

import "net/http"

func (s *Store) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /bookmarks", s.listBookmarks)
	mux.HandleFunc("POST /bookmarks", s.createBookmark)
	mux.HandleFunc("GET /bookmarks/{id}", s.getBookmark)
	mux.HandleFunc("PUT /bookmarks/{id}", s.updateBookmark)
	mux.HandleFunc("DELETE /bookmarks/{id}", s.deleteBookmark)

	mux.HandleFunc("GET /collections", s.listCollections)
	mux.HandleFunc("POST /collections", s.createCollection)
	mux.HandleFunc("GET /collections/{id}", s.getCollection)
	mux.HandleFunc("PUT /collections/{id}", s.updateCollection)
	mux.HandleFunc("DELETE /collections/{id}", s.deleteCollection)

	mux.HandleFunc("GET /collections/{id}/bookmarks", s.listCollectionBookmarks)
	mux.HandleFunc("PUT /collections/{id}/bookmarks/{bookmark_id}", s.addCollectionBookmark)
	mux.HandleFunc("DELETE /collections/{id}/bookmarks/{bookmark_id}", s.removeCollectionBookmark)
	mux.HandleFunc("PUT /collections/{id}/bookmarks/{bookmark_id}/position", s.moveCollectionBookmark)

	mux.HandleFunc("POST /bookmarks/{id}/share", s.createBookmarkShare)
	mux.HandleFunc("POST /collections/{id}/share", s.createCollectionShare)
	mux.HandleFunc("GET /public/{token}", s.getPublicShare)
	mux.HandleFunc("DELETE /public/{token}", s.revokePublicShare)
}
