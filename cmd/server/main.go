package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"depthalign/internal/server"
	"depthalign/internal/store"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:5530", "HTTP listen address")
	dbPath := flag.String("db", "depthalign.db", "SQLite database path")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer st.Close()

	srv := &http.Server{
		Addr:              *listen,
		Handler:           server.New(st),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("井深对齐台 listening on http://%s", *listen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
}
