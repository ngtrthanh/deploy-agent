package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ngtrthanh/deploy-agent/internal/desired"
)

type Observed struct {
	Exists          bool   `json:"exists"`
	ContainerID     string `json:"container_id,omitempty"`
	ImageID         string `json:"image_id,omitempty"`
	ImageRepository string `json:"image_repository,omitempty"`
	ImageDigest     string `json:"image_digest,omitempty"`
}

type Runtime interface {
	Observe(context.Context) (Observed, error)
	VerifyArtifact(context.Context, desired.Release) error
	Pull(context.Context, desired.Release) error
	Apply(context.Context, desired.Release) error
	RunMigration(context.Context, desired.Release) error
}

type Compose struct {
	Dir              string
	File             string
	Service          string
	ImageEnv         string
	Environment      string
	Instance         string
	AgentVersion     string
	MigrationCommand []string
}

func (r *Compose) Pull(ctx context.Context, rel desired.Release) error {
	cmd := exec.CommandContext(ctx, "docker", "pull", rel.ImageReference())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker pull %s: %w", rel.ImageReference(), err)
	}
	return nil
}

func (r *Compose) Apply(ctx context.Context, rel desired.Release) error {
	return r.compose(ctx, rel, "up", "-d", "--no-deps", "--force-recreate", r.Service)
}

func (r *Compose) RunMigration(ctx context.Context, rel desired.Release) error {
	if !rel.Spec.Migration.Required {
		return nil
	}
	if len(r.MigrationCommand) == 0 {
		return errors.New("migration.required=true but runtime.migration_command is empty")
	}
	cmd := exec.CommandContext(ctx, r.MigrationCommand[0], r.MigrationCommand[1:]...)
	cmd.Dir = r.Dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = r.env(rel)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("migration command failed: %w", err)
	}
	return nil
}

func (r *Compose) Observe(ctx context.Context) (Observed, error) {
	containerID, err := r.composeOutput(ctx, "ps", "-q", r.Service)
	if err != nil {
		return Observed{}, err
	}
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return Observed{Exists: false}, nil
	}

	imageID, err := commandOutput(ctx, "docker", "inspect", "--format", "{{.Image}}", containerID)
	if err != nil {
		return Observed{}, fmt.Errorf("inspect container image: %w", err)
	}
	imageID = strings.TrimSpace(imageID)
	observed := Observed{Exists: true, ContainerID: containerID, ImageID: imageID}

	rawDigests, err := commandOutput(ctx, "docker", "image", "inspect", "--format", "{{json .RepoDigests}}", imageID)
	if err != nil {
		return observed, nil
	}
	var repoDigests []string
	if json.Unmarshal([]byte(strings.TrimSpace(rawDigests)), &repoDigests) == nil && len(repoDigests) > 0 {
		ref := repoDigests[0]
		if at := strings.LastIndex(ref, "@"); at > 0 {
			observed.ImageRepository = ref[:at]
			observed.ImageDigest = strings.ToLower(strings.TrimSpace(ref[at+1:]))
		}
	}
	return observed, nil
}

func (r *Compose) VerifyArtifact(ctx context.Context, rel desired.Release) error {
	observed, err := r.Observe(ctx)
	if err != nil {
		return err
	}
	if !observed.Exists {
		return errors.New("managed service is absent")
	}
	desiredImageID, err := commandOutput(ctx, "docker", "image", "inspect", "--format", "{{.Id}}", rel.ImageReference())
	if err != nil {
		return fmt.Errorf("resolve desired artifact %s: %w", rel.ImageReference(), err)
	}
	desiredImageID = strings.TrimSpace(desiredImageID)
	if desiredImageID == "" || observed.ImageID != desiredImageID {
		return fmt.Errorf("running image id %q does not match desired artifact image id %q", observed.ImageID, desiredImageID)
	}
	return nil
}

func (r *Compose) compose(ctx context.Context, rel desired.Release, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, r.composeArgs(args...)...)...)
	cmd.Dir = r.Dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = r.env(rel)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose %v: %w", args, err)
	}
	return nil
}

func (r *Compose) composeOutput(ctx context.Context, args ...string) (string, error) {
	out, err := commandOutputWithDir(ctx, r.Dir, "docker", append([]string{"compose"}, r.composeArgs(args...)...)...)
	if err != nil {
		return "", fmt.Errorf("docker compose %v: %w", args, err)
	}
	return out, nil
}

func (r *Compose) composeArgs(args ...string) []string {
	if r.File == "" {
		return args
	}
	return append([]string{"-f", filepath.Join(r.Dir, r.File)}, args...)
}

func (r *Compose) env(rel desired.Release) []string {
	imageEnv := r.ImageEnv
	if imageEnv == "" {
		imageEnv = "DEPLOY_IMAGE"
	}
	return append(os.Environ(),
		fmt.Sprintf("%s=%s", imageEnv, rel.ImageReference()),
		"IMAGE_DIGEST="+rel.Spec.Digest,
		"APP_ENV="+r.Environment,
		"INSTANCE_ID="+r.Instance,
		"DEPLOY_AGENT_VERSION="+r.AgentVersion,
	)
}

func commandOutput(ctx context.Context, name string, args ...string) (string, error) {
	return commandOutputWithDir(ctx, "", name, args...)
}

func commandOutputWithDir(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	b, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return string(b), nil
}
