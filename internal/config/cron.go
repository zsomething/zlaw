package config

import (
	"bytes"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// CronJobConfig defines a single scheduled task.
type CronJobConfig struct {
	ID       string `toml:"id"`
	Schedule string `toml:"schedule"` // standard 5-field cron expression
	Task     string `toml:"task"`     // prompt sent to the agent
	Target   string `toml:"target"`   // push target, e.g. "telegram:123456789"
}

// CronConfig holds all cron job definitions for an agent.
// It is loaded from cron.toml in the agent directory, separate from agent.toml
// so agent tools can write to it without touching static config.
type CronConfig struct {
	Jobs []CronJobConfig `toml:"cron"`
}

// LoadCronConfig reads cron.toml from dir. A missing file is silently treated
// as an empty config (no jobs).
func LoadCronConfig(dir string) (CronConfig, error) {
	path := dir + "/cron.toml"
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return CronConfig{}, nil
	}
	if err != nil {
		return CronConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg CronConfig
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return CronConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// WriteCronConfig atomically writes cfg to cron.toml in dir (write to .tmp,
// then rename). Callers should call this from a single goroutine at a time.
func WriteCronConfig(dir string, cfg CronConfig) error {
	path := dir + "/cron.toml"
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return fmt.Errorf("encode cron.toml: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename %s → %s: %w", tmp, path, err)
	}
	return nil
}
