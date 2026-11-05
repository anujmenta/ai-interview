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
	store.registerRoutes(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("listening on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
