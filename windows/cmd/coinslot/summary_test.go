package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/redwolf3/coinslot/core/config"
	"github.com/redwolf3/coinslot/core/summary"
	"github.com/redwolf3/coinslot/windows/collector"
)

type scriptedProbe struct {
	fg    collector.Foreground
	idle  float64
	procs []collector.Proc
}

func (p *scriptedProbe) Foreground() (collector.Foreground, error)   { return p.fg, nil }
func (p *scriptedProbe) IdleSeconds() (float64, error)               { return p.idle, nil }
func (p *scriptedProbe) SessionProcesses() ([]collector.Proc, error) { return p.procs, nil }
func (p *scriptedProbe) ProcessPath(uint32) string                   { return "" }
func (p *scriptedProbe) ConsoleActive() bool                         { return true }

// End to end: the real collector loop writes a log through FileSink, and the
// summary command reads it back. Guards the writer/reader schema contract.
func TestCollectorToSummary(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfgJSON := `{"schema":1,"apps":[
	 {"app":"roblox","class":"metered","role":"player","exe":["RobloxPlayerBeta.exe"]},
	 {"app":"duolingo","class":"allowed","exe":["msedge.exe"],"title_contains":["Duolingo"]}]}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, sum, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 6, 18, 0, 0, 0, time.Local)
	probe := &scriptedProbe{}
	logDir := filepath.Join(dir, "logs")
	c := &collector.Collector{Cfg: cfg, ConfigPath: cfgPath, ConfigSHA: sum, Probe: probe,
		Sink: collector.FileSink{Dir: logDir}, ID: collector.Identity{User: `PC\kid1`, SID: "S-1-5-21-1001"},
		Now: func() time.Time { return now }}

	tick := func(n int) {
		for i := 0; i < n; i++ {
			c.Tick()
			now = now.Add(5 * time.Second)
		}
	}
	c.Start()
	probe.procs = []collector.Proc{{PID: 4, Exe: "msedge.exe"}}
	probe.fg = collector.Foreground{PID: 4, Exe: "msedge.exe", Title: "Duolingo"}
	tick(1 + 24) // first tick credits 0, then 2 min allowed
	probe.procs = append(probe.procs, collector.Proc{PID: 7, Exe: "RobloxPlayerBeta.exe"})
	probe.fg = collector.Foreground{PID: 7, Exe: "RobloxPlayerBeta.exe", Title: "Roblox"}
	tick(36) // 3 min play
	probe.idle = 90
	tick(12) // 1 min idle with game in front
	c.Stop("test")

	var out, errb bytes.Buffer
	if err := summaryCmd([]string{"-json", logDir}, cfgPath, logDir, &out, &errb); err != nil {
		t.Fatalf("%v: %s", err, errb.String())
	}
	var days []summary.Day
	if err := json.Unmarshal(out.Bytes(), &days); err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 {
		t.Fatalf("days: %s", out.String())
	}
	d := days[0]
	if d.ActivePlayMin != 3 || d.AllowedMin != 2 || d.OpenNotPlayingMin != 1 || d.User != `PC\kid1` {
		t.Fatalf("summary: %+v", d)
	}

	out.Reset()
	if err := summaryCmd([]string{logDir}, cfgPath, logDir, &out, &errb); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ACTIVE PLAY") {
		t.Fatalf("text output:\n%s", out.String())
	}
	out.Reset()
	if err := inventoryCmd([]string{logDir}, cfgPath, logDir, &out, &errb); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"RobloxPlayerBeta.exe", "roblox/metered/player", `"Roblox"`, "msedge.exe", `"Duolingo"`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("inventory missing %q:\n%s", want, out.String())
		}
	}
}
