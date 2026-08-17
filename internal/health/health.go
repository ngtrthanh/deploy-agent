package health

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ngtrthanh/deploy-agent/internal/desired"
)

type ReadyResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

type Identity struct {
	Service       string `json:"service"`
	Environment   string `json:"environment"`
	InstanceID    string `json:"instance_id,omitempty"`
	Hostname      string `json:"hostname,omitempty"`
	AppVersion    string `json:"app_version,omitempty"`
	GitSHA        string `json:"git_sha"`
	ImageDigest   string `json:"image_digest"`
	BuildTime     string `json:"build_time,omitempty"`
	StartedAt     string `json:"started_at,omitempty"`
	UptimeSeconds uint64 `json:"uptime_seconds,omitempty"`
	PID           int    `json:"pid,omitempty"`
}

type Checker interface {
	Liveness(context.Context) error
	Readiness(context.Context) (ReadyResponse, error)
	Identity(context.Context) (Identity, error)
	Verify(context.Context, desired.Release, string, string) (Identity, error)
	WaitFor(context.Context, desired.Release, string, string, time.Duration, time.Duration) (Identity, error)
}

type HTTPChecker struct {
	HealthURL         string
	ReadyURL          string
	IdentityURL       string
	IdentityTokenFile string
	Client            *http.Client
}

func NewHTTPChecker(healthURL, readyURL, identityURL, tokenFile string, timeout time.Duration) *HTTPChecker {
	return &HTTPChecker{
		HealthURL:         healthURL,
		ReadyURL:          readyURL,
		IdentityURL:       identityURL,
		IdentityTokenFile: tokenFile,
		Client:            &http.Client{Timeout: timeout},
	}
}

func (c *HTTPChecker) Liveness(ctx context.Context) error {
	resp, err := c.do(ctx, c.HealthURL, false)
	if err != nil {
		return fmt.Errorf("health request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned HTTP %d", resp.StatusCode)
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(resp.Body, &body); err != nil {
		return fmt.Errorf("decode health JSON: %w", err)
	}
	if body.Status != "ok" {
		return fmt.Errorf("health status is %q, expected ok", body.Status)
	}
	return nil
}

func (c *HTTPChecker) Readiness(ctx context.Context) (ReadyResponse, error) {
	resp, err := c.do(ctx, c.ReadyURL, false)
	if err != nil {
		return ReadyResponse{}, fmt.Errorf("ready request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ReadyResponse{}, fmt.Errorf("ready endpoint returned HTTP %d", resp.StatusCode)
	}
	var body ReadyResponse
	if err := decodeJSON(resp.Body, &body); err != nil {
		return ReadyResponse{}, fmt.Errorf("decode ready JSON: %w", err)
	}
	if body.Status != "ready" {
		return ReadyResponse{}, fmt.Errorf("ready status is %q, expected ready", body.Status)
	}
	if state := strings.ToLower(strings.TrimSpace(body.Checks["migration_state"])); state == "behind" {
		return ReadyResponse{}, fmt.Errorf("migration_state is behind")
	}
	return body, nil
}

func (c *HTTPChecker) Identity(ctx context.Context) (Identity, error) {
	resp, err := c.do(ctx, c.IdentityURL, true)
	if err != nil {
		return Identity{}, fmt.Errorf("identity request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Identity{}, fmt.Errorf("identity endpoint returned HTTP %d", resp.StatusCode)
	}
	var body Identity
	if err := decodeJSON(resp.Body, &body); err != nil {
		return Identity{}, fmt.Errorf("decode identity JSON: %w", err)
	}
	body.Service = strings.TrimSpace(body.Service)
	body.Environment = strings.TrimSpace(body.Environment)
	body.GitSHA = strings.ToLower(strings.TrimSpace(body.GitSHA))
	body.ImageDigest = strings.ToLower(strings.TrimSpace(body.ImageDigest))
	return body, nil
}

func (c *HTTPChecker) Verify(ctx context.Context, rel desired.Release, service, environment string) (Identity, error) {
	if err := c.Liveness(ctx); err != nil {
		return Identity{}, err
	}
	ready, err := c.Readiness(ctx)
	if err != nil {
		return Identity{}, err
	}
	if rel.Spec.Migration.Required {
		if state := strings.ToLower(strings.TrimSpace(ready.Checks["migration_state"])); state != "current" {
			return Identity{}, fmt.Errorf("migration_state is %q, expected current", state)
		}
	}
	id, err := c.Identity(ctx)
	if err != nil {
		return Identity{}, err
	}
	if id.Service != service {
		return Identity{}, fmt.Errorf("identity service is %q, expected %q", id.Service, service)
	}
	if id.Environment != environment {
		return Identity{}, fmt.Errorf("identity environment is %q, expected %q", id.Environment, environment)
	}
	if id.ImageDigest != rel.Spec.Digest {
		return Identity{}, fmt.Errorf("identity image_digest %q does not match desired %q", id.ImageDigest, rel.Spec.Digest)
	}
	if id.GitSHA != rel.Provenance.GitSHA {
		return Identity{}, fmt.Errorf("identity git_sha %q does not match desired provenance %q", id.GitSHA, rel.Provenance.GitSHA)
	}
	return id, nil
}

func (c *HTTPChecker) WaitFor(ctx context.Context, rel desired.Release, service, environment string, timeout, retryInterval time.Duration) (Identity, error) {
	deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var lastErr error
	for {
		id, err := c.Verify(deadlineCtx, rel, service, environment)
		if err == nil {
			return id, nil
		}
		lastErr = err
		timer := time.NewTimer(retryInterval)
		select {
		case <-deadlineCtx.Done():
			timer.Stop()
			return Identity{}, fmt.Errorf("deployment verification timed out: %w; last error: %v", deadlineCtx.Err(), lastErr)
		case <-timer.C:
		}
	}
}

func (c *HTTPChecker) do(ctx context.Context, url string, identity bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if identity && c.IdentityTokenFile != "" {
		b, err := os.ReadFile(c.IdentityTokenFile)
		if err != nil {
			return nil, fmt.Errorf("read identity token file: %w", err)
		}
		token := strings.TrimSpace(string(b))
		if token == "" {
			return nil, fmt.Errorf("identity token file is empty")
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return c.Client.Do(req)
}

func decodeJSON(r io.Reader, v any) error {
	b, err := io.ReadAll(io.LimitReader(r, 64*1024))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
