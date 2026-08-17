package desired

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

var (
	fullSHA    = regexp.MustCompile(`^[0-9a-f]{40}$`)
	fullDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type Release struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   Metadata   `json:"metadata"`
	Spec       Spec       `json:"spec"`
	Provenance Provenance `json:"provenance"`
	Promotion  Promotion  `json:"promotion,omitempty"`
}

type Metadata struct {
	Service     string `json:"service"`
	Environment string `json:"environment"`
}

type Spec struct {
	Image     string        `json:"image"`
	Digest    string        `json:"digest"`
	Rollout   RolloutSpec   `json:"rollout"`
	Migration MigrationSpec `json:"migration"`
}

type RolloutSpec struct {
	Strategy             string `json:"strategy"`
	HealthTimeoutSeconds int    `json:"health_timeout_seconds,omitempty"`
}

type MigrationSpec struct {
	Required bool `json:"required"`
}

type Provenance struct {
	GitSHA      string `json:"git_sha"`
	GitRef      string `json:"git_ref,omitempty"`
	BuiltAt     string `json:"built_at,omitempty"`
	BuildRunURL string `json:"build_run_url,omitempty"`
}

type Promotion struct {
	FromEnvironment string `json:"from_environment,omitempty"`
	PromotedBy      string `json:"promoted_by,omitempty"`
	PromotedAt      string `json:"promoted_at,omitempty"`
	ApprovalRef     string `json:"approval_ref,omitempty"`
	SoakSatisfied   bool   `json:"soak_satisfied,omitempty"`
}

func (r *Release) normalize() {
	r.APIVersion = strings.TrimSpace(r.APIVersion)
	r.Kind = strings.TrimSpace(r.Kind)
	r.Metadata.Service = strings.TrimSpace(r.Metadata.Service)
	r.Metadata.Environment = strings.TrimSpace(r.Metadata.Environment)
	r.Spec.Image = strings.TrimSpace(r.Spec.Image)
	r.Spec.Digest = strings.ToLower(strings.TrimSpace(r.Spec.Digest))
	r.Spec.Rollout.Strategy = strings.TrimSpace(r.Spec.Rollout.Strategy)
	r.Provenance.GitSHA = strings.ToLower(strings.TrimSpace(r.Provenance.GitSHA))
}

func (r Release) Validate() error {
	if r.APIVersion != "deploy/v1" {
		return fmt.Errorf("desired apiVersion must be deploy/v1, got %q", r.APIVersion)
	}
	if r.Kind != "Release" {
		return fmt.Errorf("desired kind must be Release, got %q", r.Kind)
	}
	if r.Metadata.Service == "" || r.Metadata.Environment == "" {
		return errors.New("desired metadata.service and metadata.environment are required")
	}
	if r.Spec.Image == "" {
		return errors.New("desired spec.image is required")
	}
	if !fullDigest.MatchString(r.Spec.Digest) {
		return fmt.Errorf("desired spec.digest must be a full sha256 digest, got %q", r.Spec.Digest)
	}
	if !fullSHA.MatchString(r.Provenance.GitSHA) {
		return fmt.Errorf("desired provenance.git_sha must be a full 40-character hexadecimal SHA, got %q", r.Provenance.GitSHA)
	}
	if r.Spec.Rollout.Strategy == "" {
		return errors.New("desired spec.rollout.strategy is required")
	}
	if r.Spec.Rollout.Strategy != "recreate" {
		return fmt.Errorf("v0.2 T1 supports rollout.strategy=recreate only, got %q", r.Spec.Rollout.Strategy)
	}
	return nil
}

func (r Release) ImageReference() string { return r.Spec.Image + "@" + r.Spec.Digest }
func (r Release) Key() string            { return r.Spec.Digest }

func (r Release) MatchesTarget(service, environment string) error {
	if r.Metadata.Service != service {
		return fmt.Errorf("desired service %q does not match configured app %q", r.Metadata.Service, service)
	}
	if r.Metadata.Environment != environment {
		return fmt.Errorf("desired environment %q does not match configured environment %q", r.Metadata.Environment, environment)
	}
	return nil
}

type Source interface {
	Get(context.Context) (Release, error)
}

type FileSource struct{ Path string }

func (s FileSource) Get(_ context.Context) (Release, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return Release{}, fmt.Errorf("read desired file: %w", err)
	}
	return Parse(b)
}

type HTTPSource struct {
	URL    string
	Client *http.Client
}

func (s HTTPSource) Get(ctx context.Context) (Release, error) {
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return Release{}, fmt.Errorf("build desired request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("fetch desired release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Release{}, fmt.Errorf("fetch desired release: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return Release{}, fmt.Errorf("read desired response: %w", err)
	}
	return Parse(b)
}

func Parse(b []byte) (Release, error) {
	raw := strings.TrimSpace(string(b))
	if raw == "" {
		return Release{}, errors.New("desired release is empty")
	}
	if !strings.HasPrefix(raw, "{") {
		return Release{}, errors.New("v0.2 desired state must be the JSON representation of deploy/v1 Release; legacy SHA pointers are no longer accepted")
	}
	var r Release
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return Release{}, fmt.Errorf("parse desired JSON: %w", err)
	}
	r.normalize()
	if err := r.Validate(); err != nil {
		return Release{}, err
	}
	return r, nil
}
