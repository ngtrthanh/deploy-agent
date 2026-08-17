package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ngtrthanh/deploy-agent/internal/desired"
)

type Accepted struct {
	GitSHA     string    `json:"git_sha"`
	ImageTag   string    `json:"image_tag"`
	AcceptedAt time.Time `json:"accepted_at"`
}

type Store interface {
	Load() (Accepted, error)
	Save(desired.Release) error
}

type FileStore struct{ Path string }

func (s FileStore) Load() (Accepted, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return Accepted{}, err
	}
	var a Accepted
	if err := json.Unmarshal(b, &a); err != nil {
		return Accepted{}, fmt.Errorf("parse state file: %w", err)
	}
	return a, nil
}

func (s FileStore) Save(r desired.Release) error {
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create state directory: %w", err)
	}
	a := Accepted{GitSHA: r.GitSHA, ImageTag: r.ImageTag, AcceptedAt: time.Now().UTC()}
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".deploy-agent-state-*.tmp")
	if err != nil {
		return fmt.Errorf("create state temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod state temp file: %w", err)
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("write state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close state: %w", err)
	}
	if err := os.Rename(tmpPath, s.Path); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return fmt.Errorf("commit state: %w", err)
	}

	// Windows rename-over-existing is not consistently atomic. Fall back to replace.
	if err := os.Remove(s.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("replace old state: %w", err)
	}
	if err := os.Rename(tmpPath, s.Path); err != nil {
		return fmt.Errorf("commit state after replace: %w", err)
	}
	return nil
}
