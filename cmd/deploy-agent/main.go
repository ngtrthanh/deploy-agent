package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ngtrthanh/deploy-agent/internal/agent"
	"github.com/ngtrthanh/deploy-agent/internal/config"
	"github.com/ngtrthanh/deploy-agent/internal/desired"
	"github.com/ngtrthanh/deploy-agent/internal/health"
	"github.com/ngtrthanh/deploy-agent/internal/runtime"
	"github.com/ngtrthanh/deploy-agent/internal/state"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "deploy-agent.json", "path to deploy-agent JSON config")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}
	command := "once"
	if flag.NArg() > 0 {
		command = flag.Arg(0)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	a := buildAgent(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.AgentHealth.Listen != "" && command == "run" {
		go serveAgentHealth(ctx, cfg.AgentHealth.Listen, a)
	}

	switch command {
	case "once":
		if err := a.Once(ctx); err != nil {
			log.Fatal(err)
		}
	case "check":
		err := a.Check(ctx)
		printJSON(a.Snapshot())
		if err != nil {
			os.Exit(1)
		}
	case "run":
		if err := runLoop(ctx, cfg.PollInterval(), a); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unknown command %q; use once, run, or check", command)
	}
}

func buildAgent(cfg config.Config) *agent.Agent {
	var src desired.Source
	switch cfg.Desired.Source {
	case "file":
		src = desired.FileSource{Path: cfg.Desired.Path}
	case "http":
		src = desired.HTTPSource{URL: cfg.Desired.URL}
	default:
		panic("validated unsupported desired source")
	}

	compose := &runtime.Compose{
		Dir:           cfg.Runtime.ComposeDir,
		File:          cfg.Runtime.ComposeFile,
		Service:       cfg.Runtime.Service,
		Image:         cfg.Image,
		ImageEnv:      cfg.Runtime.ImageEnv,
		ImageEnvValue: cfg.Runtime.ImageEnvValue,
	}
	statePath := cfg.StateFile
	if !filepath.IsAbs(statePath) {
		statePath = filepath.Join(cfg.Runtime.ComposeDir, statePath)
	}

	return &agent.Agent{
		App:            cfg.App,
		Environment:    cfg.Environment,
		Instance:       cfg.Instance,
		Source:         src,
		Runtime:        compose,
		Health:         health.NewHTTPChecker(cfg.Verify.URL, cfg.Verify.ExpectedService, cfg.RequestTimeout()),
		Store:          state.FileStore{Path: statePath},
		Rollback:       cfg.Rollback,
		StartupTimeout: cfg.StartupTimeout(),
		RetryInterval:  cfg.RetryInterval(),
	}
}

func runLoop(ctx context.Context, interval time.Duration, a *agent.Agent) error {
	for {
		if err := a.Once(ctx); err != nil {
			log.Printf("deployment cycle failed: %v", err)
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func serveAgentHealth(ctx context.Context, listen string, a *agent.Agent) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		s := a.Snapshot()
		status := "ok"
		if s.LastError != "" && s.DeploymentState != "rolled_back" {
			status = "degraded"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Status  string `json:"status"`
			Version string `json:"agent_version"`
			agent.Snapshot
		}{Status: status, Version: version, Snapshot: s})
	})
	srv := &http.Server{Addr: listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("agent health server failed: %v", err)
	}
}

func printJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(b))
}
