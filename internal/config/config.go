package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

type Config struct {
	App         string            `json:"app"`
	Environment string            `json:"environment"`
	Instance    string            `json:"instance"`
	Image       string            `json:"image"`
	Desired     DesiredConfig     `json:"desired"`
	Runtime     RuntimeConfig     `json:"runtime"`
	Verify      VerifyConfig      `json:"verify"`
	StateFile   string            `json:"state_file"`
	PollSeconds int               `json:"poll_seconds"`
	Rollback    bool              `json:"rollback"`
	AgentHealth AgentHealthConfig `json:"agent_health"`
}

type DesiredConfig struct {
	Source string `json:"source"`
	Path   string `json:"path"`
	URL    string `json:"url"`
}

type RuntimeConfig struct {
	Type          string `json:"type"`
	ComposeDir    string `json:"compose_dir"`
	ComposeFile   string `json:"compose_file"`
	Service       string `json:"service"`
	ImageEnv      string `json:"image_env"`
	ImageEnvValue string `json:"image_env_value"`
}

type VerifyConfig struct {
	URL              string `json:"url"`
	ExpectedService  string `json:"expected_service"`
	RequestTimeout   int    `json:"request_timeout_seconds"`
	StartupTimeout   int    `json:"startup_timeout_seconds"`
	RetryIntervalSec int    `json:"retry_interval_seconds"`
}

type AgentHealthConfig struct {
	Listen string `json:"listen"`
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse config JSON: %w", err)
	}
	cfg.setDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) setDefaults() {
	if c.Desired.Source == "" {
		c.Desired.Source = "file"
	}
	if c.Runtime.Type == "" {
		c.Runtime.Type = "docker-compose"
	}
	if c.Runtime.ComposeFile == "" {
		c.Runtime.ComposeFile = "compose.yml"
	}
	if c.Runtime.ImageEnv == "" {
		c.Runtime.ImageEnv = "DEPLOY_IMAGE"
	}
	if c.Runtime.ImageEnvValue == "" {
		c.Runtime.ImageEnvValue = "image"
	}
	if c.StateFile == "" {
		c.StateFile = ".deploy-agent-state.json"
	}
	if c.PollSeconds <= 0 {
		c.PollSeconds = 60
	}
	if c.Verify.ExpectedService == "" {
		c.Verify.ExpectedService = c.App
	}
	if c.Verify.RequestTimeout <= 0 {
		c.Verify.RequestTimeout = 5
	}
	if c.Verify.StartupTimeout <= 0 {
		c.Verify.StartupTimeout = 60
	}
	if c.Verify.RetryIntervalSec <= 0 {
		c.Verify.RetryIntervalSec = 2
	}
}

func (c Config) Validate() error {
	if c.App == "" {
		return errors.New("config: app is required")
	}
	if c.Image == "" {
		return errors.New("config: image is required")
	}
	switch c.Desired.Source {
	case "file":
		if c.Desired.Path == "" {
			return errors.New("config: desired.path is required for file source")
		}
	case "http":
		if c.Desired.URL == "" {
			return errors.New("config: desired.url is required for http source")
		}
	default:
		return fmt.Errorf("config: unsupported desired.source %q", c.Desired.Source)
	}
	if c.Runtime.Type != "docker-compose" {
		return fmt.Errorf("config: unsupported runtime.type %q", c.Runtime.Type)
	}
	if c.Runtime.ComposeDir == "" || c.Runtime.Service == "" {
		return errors.New("config: runtime.compose_dir and runtime.service are required")
	}
	if c.Runtime.ImageEnvValue != "image" && c.Runtime.ImageEnvValue != "tag" {
		return errors.New("config: runtime.image_env_value must be image or tag")
	}
	if c.Verify.URL == "" {
		return errors.New("config: verify.url is required")
	}
	return nil
}

func (c Config) PollInterval() time.Duration { return time.Duration(c.PollSeconds) * time.Second }
func (c Config) RequestTimeout() time.Duration {
	return time.Duration(c.Verify.RequestTimeout) * time.Second
}
func (c Config) StartupTimeout() time.Duration {
	return time.Duration(c.Verify.StartupTimeout) * time.Second
}
func (c Config) RetryInterval() time.Duration {
	return time.Duration(c.Verify.RetryIntervalSec) * time.Second
}
