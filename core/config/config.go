// Package config defines the Coinslot app-classification config and matching.
// It is pure and platform-independent.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const SchemaVersion = 1

const (
	ClassMetered = "metered"
	ClassAllowed = "allowed"

	RolePlayer   = "player"
	RoleLauncher = "launcher"
)

const (
	DefaultSampleIntervalSeconds = 5
	DefaultIdleThresholdSeconds  = 30
)

// Rule maps process names (and optionally window titles) to an app classification.
type Rule struct {
	App           string   `json:"app"`
	Class         string   `json:"class"`
	Role          string   `json:"role,omitempty"`
	Exe           []string `json:"exe"`
	TitleContains []string `json:"title_contains,omitempty"`
	Note          string   `json:"note,omitempty"`
}

type Config struct {
	Schema                int    `json:"schema"`
	SampleIntervalSeconds int    `json:"sample_interval_seconds"`
	IdleThresholdSeconds  int    `json:"idle_threshold_seconds"`
	Apps                  []Rule `json:"apps"`
}

// Load reads, defaults, and validates a config file. It also returns the SHA-256
// of the raw bytes so logs can record exactly which config produced them.
func Load(path string) (*Config, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	c, err := Parse(b)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	sum := sha256.Sum256(b)
	return c, hex.EncodeToString(sum[:]), nil
}

func Parse(b []byte) (*Config, error) {
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.SampleIntervalSeconds == 0 {
		c.SampleIntervalSeconds = DefaultSampleIntervalSeconds
	}
	if c.IdleThresholdSeconds == 0 {
		c.IdleThresholdSeconds = DefaultIdleThresholdSeconds
	}
	for i := range c.Apps {
		if c.Apps[i].Role == "" {
			c.Apps[i].Role = RolePlayer
		}
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) Validate() error {
	if c.Schema != SchemaVersion {
		return fmt.Errorf("unsupported config schema %d (want %d)", c.Schema, SchemaVersion)
	}
	if c.SampleIntervalSeconds < 1 || c.SampleIntervalSeconds > 60 {
		return fmt.Errorf("sample_interval_seconds must be 1..60, got %d", c.SampleIntervalSeconds)
	}
	if c.IdleThresholdSeconds < 1 {
		return fmt.Errorf("idle_threshold_seconds must be >= 1, got %d", c.IdleThresholdSeconds)
	}
	for i, r := range c.Apps {
		if r.App == "" {
			return fmt.Errorf("apps[%d]: app is required", i)
		}
		if r.Class != ClassMetered && r.Class != ClassAllowed {
			return fmt.Errorf("apps[%d] %s: class must be %q or %q", i, r.App, ClassMetered, ClassAllowed)
		}
		if r.Role != RolePlayer && r.Role != RoleLauncher {
			return fmt.Errorf("apps[%d] %s: role must be %q or %q", i, r.App, RolePlayer, RoleLauncher)
		}
		if len(r.Exe) == 0 {
			return fmt.Errorf("apps[%d] %s: at least one exe is required", i, r.App)
		}
	}
	return nil
}

// Match returns the first rule matching the exe name and window title, or nil.
// Matching is case-insensitive. Rules with TitleContains require a title match.
func (c *Config) Match(exe, title string) *Rule {
	for i := range c.Apps {
		r := &c.Apps[i]
		if !r.matchesExe(exe) {
			continue
		}
		if len(r.TitleContains) > 0 && !containsAnyFold(title, r.TitleContains) {
			continue
		}
		return r
	}
	return nil
}

// MatchProcess matches a background process by exe only. Rules that need a
// window title never match here.
func (c *Config) MatchProcess(exe string) *Rule {
	for i := range c.Apps {
		r := &c.Apps[i]
		if len(r.TitleContains) == 0 && r.matchesExe(exe) {
			return r
		}
	}
	return nil
}

// IsMeteredPlayer reports whether r is a metered game's player rule.
func (r *Rule) IsMeteredPlayer() bool {
	return r != nil && r.Class == ClassMetered && r.Role == RolePlayer
}

func (r *Rule) matchesExe(exe string) bool {
	for _, e := range r.Exe {
		if strings.EqualFold(e, exe) {
			return true
		}
	}
	return false
}

func containsAnyFold(s string, subs []string) bool {
	ls := strings.ToLower(s)
	for _, sub := range subs {
		if strings.Contains(ls, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}
