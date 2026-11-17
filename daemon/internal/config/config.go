// Package config loads and saves the daemon configuration and resolves its paths.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/jose00735/korokmote/daemon/internal/proto"
)

// Config is the content of config.yaml.
type Config struct {
	Name          string            `yaml:"name"`
	Port          int               `yaml:"port"`
	AllowPublic   bool              `yaml:"allow_public"`
	Autostart     string            `yaml:"autostart"` // user | off (elevated is set up manually, see docs)
	PairingWindow string            `yaml:"pairing_window"`
	FileRoots     []string          `yaml:"file_roots"`
	Launch        map[string]string `yaml:"launch"` // allow-listed launch targets: id -> path or URL
}

// Paths holds the on-disk locations used by the daemon.
type Paths struct {
	Dir     string // %APPDATA%\Korokmote
	LogDir  string // %LOCALAPPDATA%\Korokmote\logs
	Config  string
	Remotes string
}

// ResolvePaths returns the default paths, or paths under override when set.
func ResolvePaths(override string) (Paths, error) {
	var p Paths
	if override != "" {
		p.Dir = override
		p.LogDir = filepath.Join(override, "logs")
	} else {
		cfg, err := os.UserConfigDir()
		if err != nil {
			return p, fmt.Errorf("config dir: %w", err)
		}
		cache, err := os.UserCacheDir()
		if err != nil {
			return p, fmt.Errorf("cache dir: %w", err)
		}
		p.Dir = filepath.Join(cfg, "Korokmote")
		p.LogDir = filepath.Join(cache, "Korokmote", "logs")
	}
	p.Config = filepath.Join(p.Dir, "config.yaml")
	p.Remotes = filepath.Join(p.Dir, "remotes")
	for _, d := range []string{p.Dir, p.LogDir, p.Remotes} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return p, fmt.Errorf("create %s: %w", d, err)
		}
	}
	return p, nil
}

// Default returns the default configuration.
func Default() Config {
	host, _ := os.Hostname()
	if host == "" {
		host = "Korokmote PC"
	}
	return Config{
		Name:          host,
		Port:          proto.DefaultPort,
		Autostart:     "user",
		PairingWindow: "120s",
	}
}

// Load reads config.yaml, creating it with defaults when missing.
func Load(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, Save(path, c)
	}
	if err != nil {
		return c, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.Port <= 0 || c.Port > 65535 {
		c.Port = proto.DefaultPort
	}
	if strings.TrimSpace(c.Name) == "" {
		c.Name = Default().Name
	}
	return c, nil
}

// Save writes config.yaml.
func Save(path string, c Config) error {
	b, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	header := "# Korokmote daemon configuration. See docs/DAEMON.md.\n"
	if err := os.WriteFile(path, append([]byte(header), b...), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// PairingDuration parses PairingWindow with a safe default.
func (c Config) PairingDuration() time.Duration {
	d, err := time.ParseDuration(c.PairingWindow)
	if err != nil || d < 15*time.Second || d > 15*time.Minute {
		return 120 * time.Second
	}
	return d
}
