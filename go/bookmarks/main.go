package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	db, err := openDB()
	if err != nil {
		log.Fatal(err)
	}
	store := &Store{db: db}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /bookmarks", store.listBookmarks)
	mux.HandleFunc("POST /bookmarks", store.createBookmark)
	mux.HandleFunc("GET /bookmarks/{id}", store.getBookmark)
	mux.HandleFunc("PUT /bookmarks/{id}", store.updateBookmark)
	mux.HandleFunc("DELETE /bookmarks/{id}", store.deleteBookmark)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("listening on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
