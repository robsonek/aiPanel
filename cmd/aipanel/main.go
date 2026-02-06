package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	aipanel "github.com/robsonek/aiPanel"
)

func newHandler() http.Handler {
	distFS, err := fs.Sub(aipanel.FrontendFS, "web/dist")
	if err != nil {
		log.Fatalf("failed to create sub filesystem: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(distFS)))
	return mux
}

func main() {
	fmt.Println("aiPanel starting...")

	addr := ":8080"
	fmt.Printf("Listening on %s\n", addr)

	srv := &http.Server{
		Addr:         addr,
		Handler:      newHandler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
