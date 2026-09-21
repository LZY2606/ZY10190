package main

import (
	"flag"
	"log"
	"net/http"

	"wellalign/internal/store"
	"wellalign/internal/web"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:5530", "HTTP listen address")
	database := flag.String("db", "wellalign.db", "SQLite database path")
	flag.Parse()

	db, err := store.Open(*database)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	log.Printf("井深对齐台 listening on http://%s", *listen)
	if err := http.ListenAndServe(*listen, web.New(db)); err != nil {
		log.Fatal(err)
	}
}
