package health

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ngtrthanh/deploy-agent/internal/desired"
)

const (
	healthSHA    = "7bfe1179656431d785142ffb1afba3a6001a30f1"
	healthDigest = "sha256:9f2c4b1d8e5a3c7f0b6d2e9a4c8f1b5d3e7a9c2f6b0d4e8a1c5f9b3d7e2a6c40"
)

func healthRelease() desired.Release {
	return desired.Release{
		APIVersion: "deploy/v1", Kind: "Release",
		Metadata: desired.Metadata{Service: "app", Environment: "prod"},
		Spec: desired.Spec{Image: "ghcr.io/acme/app", Digest: healthDigest, Rollout: desired.RolloutSpec{Strategy: "recreate"}},
		Provenance: desired.Provenance{GitSHA: healthSHA},
	}
}

func TestVerifyHealthReadyIdentity(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"status":"ok"}`) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"status":"ready","checks":{"migration_state":"current"}}`) })
	mux.HandleFunc("/api/ops/identity", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"service":"app","environment":"prod","git_sha":"%s","image_digest":"%s"}`, healthSHA, healthDigest)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewHTTPChecker(srv.URL+"/healthz", srv.URL+"/readyz", srv.URL+"/api/ops/identity", "", time.Second)
	if _, err := c.Verify(context.Background(), healthRelease(), "app", "prod"); err != nil {
		t.Fatal(err)
	}
}

func TestRejectIdentityDigestMismatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"status":"ok"}`) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"status":"ready"}`) })
	mux.HandleFunc("/api/ops/identity", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"service":"app","environment":"prod","git_sha":"%s","image_digest":"sha256:%064d"}`, healthSHA, 0)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewHTTPChecker(srv.URL+"/healthz", srv.URL+"/readyz", srv.URL+"/api/ops/identity", "", time.Second)
	if _, err := c.Verify(context.Background(), healthRelease(), "app", "prod"); err == nil {
		t.Fatal("expected digest mismatch")
	}
}

func TestMigrationBehindFailsReadiness(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"status":"ready","checks":{"migration_state":"behind"}}`) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewHTTPChecker(srv.URL, srv.URL+"/readyz", srv.URL, "", time.Second)
	if _, err := c.Readiness(context.Background()); err == nil {
		t.Fatal("expected migration behind to fail")
	}
}
