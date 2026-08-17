package main

import (
	"encoding/json"
	"net/http"
	"os"
	"time"
)

var started = time.Now()

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("deploy-agent demo\n"))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":         "ok",
			"service":        "deploy-agent-demo",
			"git_sha":        os.Getenv("APP_GIT_SHA"),
			"image_tag":      os.Getenv("APP_IMAGE_TAG"),
			"uptime_seconds": uint64(time.Since(started).Seconds()),
		})
	})
	_ = http.ListenAndServe(":8080", mux)
}
