package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ngtrthanh/deploy-agent/internal/desired"
	"github.com/ngtrthanh/deploy-agent/internal/health"
	"github.com/ngtrthanh/deploy-agent/internal/state"
)

const (
	shaOld = "1111111111111111111111111111111111111111"
	shaNew = "2222222222222222222222222222222222222222"
)

type fakeSource struct{ rel desired.Release }

func (f fakeSource) Get(context.Context) (desired.Release, error) { return f.rel, nil }

type fakeRuntime struct {
	pulls   []desired.Release
	applies []desired.Release
	failNew bool
}

func (f *fakeRuntime) Pull(_ context.Context, r desired.Release) error {
	f.pulls = append(f.pulls, r)
	return nil
}
func (f *fakeRuntime) Apply(_ context.Context, r desired.Release) error {
	f.applies = append(f.applies, r)
	if f.failNew && r.GitSHA == shaNew {
		return errors.New("apply boom")
	}
	return nil
}

type fakeHealth struct {
	current health.Response
	failNew bool
}

func (f *fakeHealth) Read(context.Context) (health.Response, error) { return f.current, nil }
func (f *fakeHealth) WaitFor(_ context.Context, sha string, _, _ time.Duration) (health.Response, error) {
	if f.failNew && sha == shaNew {
		return health.Response{}, errors.New("wrong sha")
	}
	f.current.GitSHA = sha
	f.current.ImageTag = "sha-" + sha[:12]
	return f.current, nil
}

type memoryStore struct {
	accepted state.Accepted
	have     bool
}

func (m *memoryStore) Load() (state.Accepted, error) {
	if !m.have {
		return state.Accepted{}, errors.New("missing")
	}
	return m.accepted, nil
}
func (m *memoryStore) Save(r desired.Release) error {
	m.accepted = state.Accepted{GitSHA: r.GitSHA, ImageTag: r.ImageTag}
	m.have = true
	return nil
}

func TestOnceDeploysAndAccepts(t *testing.T) {
	rt := &fakeRuntime{}
	h := &fakeHealth{current: health.Response{Status: "ok", Service: "app", GitSHA: shaOld, ImageTag: "sha-" + shaOld[:12]}}
	st := &memoryStore{}
	a := &Agent{
		App: "app", Source: fakeSource{desired.Release{GitSHA: shaNew, ImageTag: "sha-" + shaNew[:12]}},
		Runtime: rt, Health: h, Store: st, Rollback: true, StartupTimeout: time.Second, RetryInterval: time.Millisecond,
	}
	if err := a.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !st.have || st.accepted.GitSHA != shaNew {
		t.Fatalf("new release not accepted: %+v", st.accepted)
	}
	if len(rt.applies) != 1 || rt.applies[0].GitSHA != shaNew {
		t.Fatalf("unexpected applies: %+v", rt.applies)
	}
}

func TestOnceRollsBackAfterVerificationFailure(t *testing.T) {
	rt := &fakeRuntime{}
	h := &fakeHealth{current: health.Response{Status: "ok", Service: "app", GitSHA: shaOld, ImageTag: "sha-" + shaOld[:12]}, failNew: true}
	st := &memoryStore{accepted: state.Accepted{GitSHA: shaOld, ImageTag: "sha-" + shaOld[:12]}, have: true}
	a := &Agent{
		App: "app", Source: fakeSource{desired.Release{GitSHA: shaNew, ImageTag: "sha-" + shaNew[:12]}},
		Runtime: rt, Health: h, Store: st, Rollback: true, StartupTimeout: time.Second, RetryInterval: time.Millisecond,
	}
	if err := a.Once(context.Background()); err == nil {
		t.Fatal("expected deployment failure")
	}
	if len(rt.applies) != 2 || rt.applies[1].GitSHA != shaOld {
		t.Fatalf("rollback not applied: %+v", rt.applies)
	}
	if st.accepted.GitSHA != shaOld {
		t.Fatalf("rollback not persisted: %+v", st.accepted)
	}
	if a.Snapshot().DeploymentState != "rolled_back" {
		t.Fatalf("unexpected state: %+v", a.Snapshot())
	}
}

func TestCheckReturnsErrorOnDrift(t *testing.T) {
	h := &fakeHealth{current: health.Response{Status: "ok", Service: "app", GitSHA: shaOld, ImageTag: "sha-" + shaOld[:12]}}
	a := &Agent{App: "app", Source: fakeSource{desired.Release{GitSHA: shaNew, ImageTag: "sha-" + shaNew[:12]}}, Health: h}
	if err := a.Check(context.Background()); err == nil {
		t.Fatal("expected drift error")
	}
}
