package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/ngtrthanh/deploy-agent/internal/desired"
	"github.com/ngtrthanh/deploy-agent/internal/health"
	"github.com/ngtrthanh/deploy-agent/internal/runtime"
	"github.com/ngtrthanh/deploy-agent/internal/state"
)

type Snapshot struct {
	App             string    `json:"app"`
	Environment     string    `json:"environment,omitempty"`
	Instance        string    `json:"instance,omitempty"`
	DesiredSHA      string    `json:"desired_sha,omitempty"`
	RunningSHA      string    `json:"running_sha,omitempty"`
	AcceptedSHA     string    `json:"accepted_sha,omitempty"`
	DeploymentState string    `json:"deployment_state"`
	LastCheckAt     time.Time `json:"last_check_at,omitempty"`
	LastError       string    `json:"last_error,omitempty"`
}

type Agent struct {
	App            string
	Environment    string
	Instance       string
	Source         desired.Source
	Runtime        runtime.Runtime
	Health         health.Checker
	Store          state.Store
	Rollback       bool
	StartupTimeout time.Duration
	RetryInterval  time.Duration

	mu       sync.RWMutex
	snapshot Snapshot
}

func (a *Agent) Snapshot() Snapshot {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.snapshot
}

func (a *Agent) setSnapshot(update func(*Snapshot)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.snapshot.App == "" {
		a.snapshot.App = a.App
		a.snapshot.Environment = a.Environment
		a.snapshot.Instance = a.Instance
	}
	update(&a.snapshot)
}

func (a *Agent) Check(ctx context.Context) error {
	rel, err := a.Source.Get(ctx)
	if err != nil {
		a.recordError("desired_unavailable", err)
		return err
	}
	h, err := a.Health.Read(ctx)
	if err != nil {
		a.setSnapshot(func(s *Snapshot) {
			s.DesiredSHA = rel.GitSHA
			s.LastCheckAt = time.Now().UTC()
			s.DeploymentState = "unhealthy"
			s.LastError = err.Error()
		})
		return err
	}

	a.setSnapshot(func(s *Snapshot) {
		s.DesiredSHA = rel.GitSHA
		s.RunningSHA = h.GitSHA
		s.LastCheckAt = time.Now().UTC()
		s.LastError = ""
		if h.GitSHA == rel.GitSHA {
			s.DeploymentState = "in_sync"
		} else {
			s.DeploymentState = "drift"
		}
	})
	if h.GitSHA != rel.GitSHA {
		return fmt.Errorf("deployment drift: running %s, desired %s", h.GitSHA, rel.GitSHA)
	}
	return nil
}

func (a *Agent) Once(ctx context.Context) error {
	rel, err := a.Source.Get(ctx)
	if err != nil {
		a.recordError("desired_unavailable", err)
		return err
	}
	a.setSnapshot(func(s *Snapshot) {
		s.DesiredSHA = rel.GitSHA
		s.LastCheckAt = time.Now().UTC()
		s.DeploymentState = "checking"
		s.LastError = ""
	})

	currentHealth, currentErr := a.Health.Read(ctx)
	if currentErr == nil {
		a.setSnapshot(func(s *Snapshot) { s.RunningSHA = currentHealth.GitSHA })
		if currentHealth.GitSHA == rel.GitSHA {
			if err := a.Store.Save(rel); err != nil {
				a.recordError("state_write_failed", err)
				return err
			}
			a.setSnapshot(func(s *Snapshot) {
				s.AcceptedSHA = rel.GitSHA
				s.DeploymentState = "accepted"
			})
			log.Printf("deployment already in sync: %s", rel.GitSHA)
			return nil
		}
	}

	previous, havePrevious := a.previousRelease(currentHealth, currentErr)

	a.setSnapshot(func(s *Snapshot) { s.DeploymentState = "pulling" })
	if err := a.Runtime.Pull(ctx, rel); err != nil {
		a.recordError("pull_failed", err)
		return err
	}

	a.setSnapshot(func(s *Snapshot) { s.DeploymentState = "deploying" })
	if err := a.Runtime.Apply(ctx, rel); err != nil {
		a.recordError("apply_failed", err)
		return a.failAndMaybeRollback(ctx, rel, previous, havePrevious, fmt.Errorf("apply desired release: %w", err))
	}

	a.setSnapshot(func(s *Snapshot) { s.DeploymentState = "verifying" })
	verified, verifyErr := a.Health.WaitFor(ctx, rel.GitSHA, a.StartupTimeout, a.RetryInterval)
	if verifyErr != nil {
		a.recordError("verification_failed", verifyErr)
		return a.failAndMaybeRollback(ctx, rel, previous, havePrevious, verifyErr)
	}

	if err := a.Store.Save(rel); err != nil {
		a.recordError("state_write_failed", err)
		return err
	}
	a.setSnapshot(func(s *Snapshot) {
		s.RunningSHA = verified.GitSHA
		s.AcceptedSHA = rel.GitSHA
		s.DeploymentState = "accepted"
		s.LastError = ""
	})
	log.Printf("deployment accepted: %s", rel.GitSHA)
	return nil
}

func (a *Agent) failAndMaybeRollback(ctx context.Context, failed, previous desired.Release, havePrevious bool, cause error) error {
	if !a.Rollback || !havePrevious || previous.GitSHA == "" || previous.ImageTag == "" || previous.GitSHA == failed.GitSHA {
		return cause
	}

	log.Printf("deployment failed; rolling back to %s", previous.GitSHA)
	a.setSnapshot(func(s *Snapshot) { s.DeploymentState = "rolling_back" })
	if err := a.Runtime.Pull(ctx, previous); err != nil {
		rollbackErr := fmt.Errorf("rollback pull failed: %w", err)
		a.recordError("rollback_failed", rollbackErr)
		return errors.Join(cause, rollbackErr)
	}
	if err := a.Runtime.Apply(ctx, previous); err != nil {
		rollbackErr := fmt.Errorf("rollback apply failed: %w", err)
		a.recordError("rollback_failed", rollbackErr)
		return errors.Join(cause, rollbackErr)
	}
	rolledBack, err := a.Health.WaitFor(ctx, previous.GitSHA, a.StartupTimeout, a.RetryInterval)
	if err != nil {
		rollbackErr := fmt.Errorf("rollback verification failed: %w", err)
		a.recordError("rollback_failed", rollbackErr)
		return errors.Join(cause, rollbackErr)
	}
	if err := a.Store.Save(previous); err != nil {
		a.recordError("rollback_state_write_failed", err)
		return errors.Join(cause, err)
	}
	a.setSnapshot(func(s *Snapshot) {
		s.RunningSHA = rolledBack.GitSHA
		s.AcceptedSHA = previous.GitSHA
		s.DeploymentState = "rolled_back"
		s.LastError = cause.Error()
	})
	return fmt.Errorf("deployment %s failed and was rolled back to %s: %w", failed.GitSHA, previous.GitSHA, cause)
}

func (a *Agent) previousRelease(current health.Response, currentErr error) (desired.Release, bool) {
	accepted, err := a.Store.Load()
	if err == nil && accepted.GitSHA != "" && accepted.ImageTag != "" {
		a.setSnapshot(func(s *Snapshot) { s.AcceptedSHA = accepted.GitSHA })
		return desired.Release{GitSHA: accepted.GitSHA, ImageTag: accepted.ImageTag}, true
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("warning: cannot load accepted state: %v", err)
	}
	if currentErr == nil && current.GitSHA != "" && current.ImageTag != "" {
		return desired.Release{GitSHA: current.GitSHA, ImageTag: current.ImageTag}, true
	}
	return desired.Release{}, false
}

func (a *Agent) recordError(stateName string, err error) {
	a.setSnapshot(func(s *Snapshot) {
		s.DeploymentState = stateName
		s.LastCheckAt = time.Now().UTC()
		s.LastError = err.Error()
	})
}
