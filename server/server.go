// Package server/server.go
package server

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"

	"lauzhack-bot/data"
)

var (
	storePath string
	mu        sync.Mutex
)

func Start(path string, addr string) {
	storePath = path

	http.HandleFunc("/api/schedule", handleSchedule)
	http.Handle("/", http.FileServer(http.Dir("server/ui")))

	go func() {
		if err := http.ListenAndServe(addr, nil); err != nil {
			panic(err)
		}
	}()
}

func handleSchedule(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(storePath)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(data)
		if err != nil {
			return
		}

	case http.MethodPost:
		var updated data.Store
		if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
			http.Error(w, "invalid json", 400)
			return
		}
		b, _ := json.MarshalIndent(updated, "", "  ")
		if err := os.WriteFile(storePath, b, 0o644); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		_, err := w.Write([]byte("ok"))
		if err != nil {
			return
		}

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
