package health

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReadValidHealth(t *testing.T) {
	sha := "7bfe1179656431d785142ffb1afba3a6001a30f1"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","service":"matflow","git_sha":"%s","image_tag":"sha-7bfe11796564","uptime_seconds":10}`, sha)
	}))
	defer srv.Close()

	c := NewHTTPChecker(srv.URL, "matflow", time.Second)
	h, err := c.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if h.GitSHA != sha {
		t.Fatalf("unexpected SHA %s", h.GitSHA)
	}
}

func TestRejectWrongService(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"status":"ok","service":"wrong","git_sha":"7bfe1179656431d785142ffb1afba3a6001a30f1","image_tag":"sha-7bfe11796564","uptime_seconds":1}`)
	}))
	defer srv.Close()

	c := NewHTTPChecker(srv.URL, "matflow", time.Second)
	if _, err := c.Read(context.Background()); err == nil {
		t.Fatal("expected wrong service to fail")
	}
}
