package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testJSON = `{
  "schema": 1,
  "apps": [
    {"app": "roblox", "class": "metered", "role": "player", "exe": ["RobloxPlayerBeta.exe"]},
    {"app": "roblox", "class": "metered", "role": "launcher", "exe": ["RobloxPlayerInstaller.exe"]},
    {"app": "minecraft-java", "class": "metered", "exe": ["javaw.exe"], "title_contains": ["Minecraft"]},
    {"app": "duolingo", "class": "allowed", "exe": ["msedge.exe"], "title_contains": ["Duolingo"]}
  ]
}`

func mustParse(t *testing.T, s string) *Config {
	t.Helper()
	c, err := Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDefaults(t *testing.T) {
	c := mustParse(t, testJSON)
	if c.SampleIntervalSeconds != DefaultSampleIntervalSeconds || c.IdleThresholdSeconds != DefaultIdleThresholdSeconds {
		t.Fatalf("defaults not applied: %+v", c)
	}
	if c.Apps[2].Role != RolePlayer {
		t.Fatalf("role default: got %q", c.Apps[2].Role)
	}
}

func TestMatch(t *testing.T) {
	c := mustParse(t, testJSON)
	cases := []struct {
		exe, title, wantApp, wantRole string
	}{
		{"robloxplayerbeta.EXE", "Roblox", "roblox", RolePlayer},
		{"RobloxPlayerInstaller.exe", "", "roblox", RoleLauncher},
		{"javaw.exe", "Minecraft 1.21.4", "minecraft-java", RolePlayer},
		{"javaw.exe", "Some IDE", "", ""},
		{"msedge.exe", "Duolingo - The world's best way to learn", "duolingo", RolePlayer},
		{"msedge.exe", "YouTube", "", ""},
		{"explorer.exe", "", "", ""},
	}
	for _, tc := range cases {
		r := c.Match(tc.exe, tc.title)
		got, role := "", ""
		if r != nil {
			got, role = r.App, r.Role
		}
		if got != tc.wantApp || role != tc.wantRole {
			t.Errorf("Match(%q,%q) = %q/%q, want %q/%q", tc.exe, tc.title, got, role, tc.wantApp, tc.wantRole)
		}
	}
}

func TestMatchProcessIgnoresTitleRules(t *testing.T) {
	c := mustParse(t, testJSON)
	if r := c.MatchProcess("javaw.exe"); r != nil {
		t.Fatalf("title-qualified rule matched background process: %+v", r)
	}
	if r := c.MatchProcess("ROBLOXPLAYERBETA.exe"); !r.IsMeteredPlayer() {
		t.Fatalf("expected metered player, got %+v", r)
	}
	if r := c.MatchProcess("RobloxPlayerInstaller.exe"); r.IsMeteredPlayer() {
		t.Fatal("launcher reported as player")
	}
}

func TestValidate(t *testing.T) {
	bad := map[string]string{
		"schema":   `{"schema": 2, "apps": []}`,
		"class":    `{"schema": 1, "apps": [{"app": "x", "class": "blocked", "exe": ["x.exe"]}]}`,
		"role":     `{"schema": 1, "apps": [{"app": "x", "class": "metered", "role": "updater", "exe": ["x.exe"]}]}`,
		"exe":      `{"schema": 1, "apps": [{"app": "x", "class": "metered"}]}`,
		"app":      `{"schema": 1, "apps": [{"class": "metered", "exe": ["x.exe"]}]}`,
		"interval": `{"schema": 1, "sample_interval_seconds": 120, "apps": []}`,
	}
	for name, s := range bad {
		if _, err := Parse([]byte(s)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestExampleConfigLoads(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	p := filepath.Join(filepath.Dir(file), "..", "..", "config", "config.example.json")
	c, sum, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(sum) != 64 || len(c.Apps) == 0 {
		t.Fatalf("unexpected: sum=%q apps=%d", sum, len(c.Apps))
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "UNVERIFIED") {
		t.Fatal("example config should flag unverified candidates")
	}
}
