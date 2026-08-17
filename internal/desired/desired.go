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

var fullSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

type Release struct {
	GitSHA   string `json:"git_sha" yaml:"git_sha"`
	ImageTag string `json:"image_tag" yaml:"image_tag"`
}

func (r Release) Validate() error {
	if !fullSHA.MatchString(r.GitSHA) {
		return fmt.Errorf("desired git_sha must be a full 40-character hexadecimal SHA, got %q", r.GitSHA)
	}
	if r.ImageTag == "" {
		return errors.New("desired image_tag is empty")
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
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
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

	var r Release
	if strings.HasPrefix(raw, "{") {
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			return Release{}, fmt.Errorf("parse desired JSON: %w", err)
		}
	} else {
		r.GitSHA = raw
		if fullSHA.MatchString(r.GitSHA) {
			r.ImageTag = "sha-" + strings.ToLower(r.GitSHA[:12])
		}
	}
	r.GitSHA = strings.ToLower(strings.TrimSpace(r.GitSHA))
	r.ImageTag = strings.TrimSpace(r.ImageTag)
	if err := r.Validate(); err != nil {
		return Release{}, err
	}
	return r, nil
}
