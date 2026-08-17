package health

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Response struct {
	Status        string `json:"status"`
	Service       string `json:"service"`
	Environment   string `json:"environment,omitempty"`
	Instance      string `json:"instance,omitempty"`
	Version       string `json:"version,omitempty"`
	GitSHA        string `json:"git_sha"`
	ImageTag      string `json:"image_tag"`
	UptimeSeconds uint64 `json:"uptime_seconds"`
}

type Checker interface {
	Read(context.Context) (Response, error)
	WaitFor(context.Context, string, time.Duration, time.Duration) (Response, error)
}

type HTTPChecker struct {
	URL             string
	ExpectedService string
	Client          *http.Client
}

func NewHTTPChecker(url, expectedService string, timeout time.Duration) *HTTPChecker {
	return &HTTPChecker{
		URL:             url,
		ExpectedService: expectedService,
		Client:          &http.Client{Timeout: timeout},
	}
}

func (c *HTTPChecker) Read(ctx context.Context) (Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return Response{}, fmt.Errorf("build health request: %w", err)
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("health request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("health endpoint returned HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return Response{}, fmt.Errorf("read health response: %w", err)
	}
	var h Response
	if err := json.Unmarshal(b, &h); err != nil {
		return Response{}, fmt.Errorf("decode health JSON: %w", err)
	}
	h.GitSHA = strings.ToLower(strings.TrimSpace(h.GitSHA))
	if h.Status != "ok" {
		return Response{}, fmt.Errorf("health status is %q, expected ok", h.Status)
	}
	if c.ExpectedService != "" && h.Service != c.ExpectedService {
		return Response{}, fmt.Errorf("health service is %q, expected %q", h.Service, c.ExpectedService)
	}
	if h.GitSHA == "" || h.GitSHA == "unknown" {
		return Response{}, fmt.Errorf("health git_sha is not usable: %q", h.GitSHA)
	}
	return h, nil
}

func (c *HTTPChecker) WaitFor(ctx context.Context, expectedSHA string, timeout, retryInterval time.Duration) (Response, error) {
	deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var lastErr error
	for {
		h, err := c.Read(deadlineCtx)
		if err == nil {
			if h.GitSHA == strings.ToLower(expectedSHA) {
				return h, nil
			}
			lastErr = fmt.Errorf("running git_sha %s does not match desired %s", h.GitSHA, expectedSHA)
		} else {
			lastErr = err
		}

		select {
		case <-deadlineCtx.Done():
			return Response{}, fmt.Errorf("deployment verification timed out: %w; last error: %v", deadlineCtx.Err(), lastErr)
		case <-time.After(retryInterval):
		}
	}
}
