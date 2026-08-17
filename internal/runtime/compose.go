package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ngtrthanh/deploy-agent/internal/desired"
)

type Runtime interface {
	Pull(context.Context, desired.Release) error
	Apply(context.Context, desired.Release) error
}

type Compose struct {
	Dir           string
	File          string
	Service       string
	Image         string
	ImageEnv      string
	ImageEnvValue string
}

func (r *Compose) Pull(ctx context.Context, rel desired.Release) error {
	return r.run(ctx, rel, "pull", r.Service)
}

func (r *Compose) Apply(ctx context.Context, rel desired.Release) error {
	return r.run(ctx, rel, "up", "-d", "--no-deps", "--force-recreate", r.Service)
}

func (r *Compose) run(ctx context.Context, rel desired.Release, args ...string) error {
	if r.File != "" {
		args = append([]string{"-f", filepath.Join(r.Dir, r.File)}, args...)
	}
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)
	cmd.Dir = r.Dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%s", r.ImageEnv, r.imageValue(rel)))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose %v: %w", args, err)
	}
	return nil
}

func (r *Compose) imageValue(rel desired.Release) string {
	if r.ImageEnvValue == "tag" {
		return rel.ImageTag
	}
	return r.Image + ":" + rel.ImageTag
}
