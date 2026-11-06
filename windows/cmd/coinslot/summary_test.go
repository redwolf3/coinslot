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

// Copilot review r4496152987: Minecraft Java is identified only by its window
// title. When it is left open in the background across a run boundary (here:
// local midnight), the next day's log alone must still count it as open.
func TestTitledAppOpenAcrossMidnightEndToEnd(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfgJSON := `{"schema":1,"apps":[
	 {"app":"minecraft","class":"metered","role":"player","exe":["javaw.exe"],"title_contains":["Minecraft"]},
	 {"app":"duolingo","class":"allowed","exe":["msedge.exe"],"title_contains":["Duolingo"]}]}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, sum, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 6, 23, 58, 0, 0, time.Local)
	probe := &scriptedProbe{procs: []collector.Proc{{PID: 4, Exe: "msedge.exe"}, {PID: 9, Exe: "javaw.exe"}}}
	logDir := filepath.Join(dir, "logs")
	c := &collector.Collector{Cfg: cfg, ConfigPath: cfgPath, ConfigSHA: sum, Probe: probe,
		Sink: collector.FileSink{Dir: logDir}, ID: collector.Identity{User: `PC\kid1`, SID: "S-1"},
		Now: func() time.Time { return now }}
	tick := func(n int) {
		for i := 0; i < n; i++ {
			c.Tick()
			now = now.Add(5 * time.Second)
		}
	}
	probe.fg = collector.Foreground{PID: 9, Exe: "javaw.exe", Title: "Minecraft 1.21.4 - Singleplayer"}
	tick(12) // 23:58-23:59 playing
	probe.fg = collector.Foreground{PID: 4, Exe: "msedge.exe", Title: "Duolingo"}
	tick(12 + 24) // 23:59-00:02, Minecraft open behind Duolingo across midnight

	day2 := filepath.Join(logDir, "2026-11-07.jsonl")
	var out, errb bytes.Buffer
	if err := summaryCmd([]string{"-json", day2}, cfgPath, logDir, &out, &errb); err != nil {
		t.Fatalf("%v: %s", err, errb.String())
	}
	var days []summary.Day
	if err := json.Unmarshal(out.Bytes(), &days); err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 || len(days[0].Games) != 1 || days[0].Games[0].App != "minecraft" {
		t.Fatalf("day 2 summary: %s", out.String())
	}
	// Day 2 ticks: 00:00:00..00:01:55 (24). The first starts the new run and
	// credits 0, so 23 x 5s = 115s.
	if got := days[0].Games[0].OpenNotPlayingMin; got != 1.92 {
		t.Fatalf("minecraft open on day 2 = %v min, want 1.92", got)
	}
}

type titledScenario struct {
	t      *testing.T
	dir    string
	cfg    string
	logDir string
	now    time.Time
	probe  *scriptedProbe
	c      *collector.Collector
}

func newTitled(t *testing.T, titleRule string, start time.Time) *titledScenario {
	t.Helper()
	sc := &titledScenario{t: t, dir: t.TempDir(), now: start, probe: &scriptedProbe{}}
	sc.cfg = filepath.Join(sc.dir, "config.json")
	cfgJSON := `{"schema":1,"apps":[
	 {"app":"minecraft","class":"metered","role":"player","exe":["javaw.exe"],"title_contains":["` + titleRule + `"]},
	 {"app":"duolingo","class":"allowed","exe":["msedge.exe"],"title_contains":["Duolingo"]}]}`
	if err := os.WriteFile(sc.cfg, []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, sum, err := config.Load(sc.cfg)
	if err != nil {
		t.Fatal(err)
	}
	sc.logDir = filepath.Join(sc.dir, "logs")
	sc.c = &collector.Collector{Cfg: cfg, ConfigPath: sc.cfg, ConfigSHA: sum, Probe: sc.probe,
		Sink: collector.FileSink{Dir: sc.logDir}, ID: collector.Identity{User: `PC\kid1`, SID: "S-1"},
		Now: func() time.Time { return sc.now }}
	return sc
}

func (sc *titledScenario) tick(n int) {
	for i := 0; i < n; i++ {
		sc.c.Tick()
		sc.now = sc.now.Add(5 * time.Second)
	}
}

// minecraftOpen summarises the given log file and returns Minecraft's
// open-not-playing minutes.
func (sc *titledScenario) minecraftOpen(file string) float64 {
	sc.t.Helper()
	var out, errb bytes.Buffer
	if err := summaryCmd([]string{"-json", filepath.Join(sc.logDir, file)}, sc.cfg, sc.logDir, &out, &errb); err != nil {
		sc.t.Fatalf("%v: %s", err, errb.String())
	}
	var days []summary.Day
	if err := json.Unmarshal(out.Bytes(), &days); err != nil {
		sc.t.Fatal(err)
	}
	for _, d := range days {
		for _, g := range d.Games {
			if g.App == "minecraft" {
				return g.OpenNotPlayingMin
			}
		}
	}
	return 0
}

// Copilot review r4496231028: with a "Singleplayer" title rule, the process later
// shows a non-matching menu title before midnight; the next day must still count
// it as open.
func TestTitleChangeBeforeRunBoundaryEndToEnd(t *testing.T) {
	sc := newTitled(t, "Singleplayer", time.Date(2026, 11, 6, 23, 58, 0, 0, time.Local))
	sc.probe.procs = []collector.Proc{{PID: 4, Exe: "msedge.exe"}, {PID: 9, Exe: "javaw.exe"}}
	sc.probe.fg = collector.Foreground{PID: 9, Exe: "javaw.exe", Title: "Minecraft 1.21.4 - Singleplayer"}
	sc.tick(6)
	sc.probe.fg = collector.Foreground{PID: 9, Exe: "javaw.exe", Title: "Minecraft 1.21.4"} // back in a menu
	sc.tick(6)
	sc.probe.fg = collector.Foreground{PID: 4, Exe: "msedge.exe", Title: "Duolingo"}
	sc.tick(12 + 24) // across midnight
	if got := sc.minecraftOpen("2026-11-07.jsonl"); got != 1.92 {
		t.Fatalf("minecraft open on day 2 = %v, want 1.92", got)
	}
}

// Copilot review 5805169230 (activity.go:147): the game is seen in the foreground
// before a process snapshot includes it; the later start event must not erase
// the identity learned from its title.
func TestForegroundBeforeSnapshotEndToEnd(t *testing.T) {
	sc := newTitled(t, "Minecraft", time.Date(2026, 11, 6, 18, 0, 0, 0, time.Local))
	sc.probe.procs = []collector.Proc{{PID: 4, Exe: "msedge.exe"}} // snapshot taken just before javaw started
	sc.probe.fg = collector.Foreground{PID: 9, Exe: "javaw.exe", Title: "Minecraft 1.21.4"}
	sc.tick(2)
	sc.probe.procs = []collector.Proc{{PID: 4, Exe: "msedge.exe"}, {PID: 9, Exe: "javaw.exe"}}
	sc.probe.fg = collector.Foreground{PID: 4, Exe: "msedge.exe", Title: "Duolingo"}
	sc.tick(12)
	if got := sc.minecraftOpen("2026-11-06.jsonl"); got != 1 {
		t.Fatalf("minecraft open = %v, want 1", got)
	}
}

// Issue #2: with -date, a log directory contributes only that day's file.
func TestSummaryDateReadsOnlyThatDaysFile(t *testing.T) {
	sc := newTitled(t, "Minecraft", time.Date(2026, 11, 6, 18, 0, 0, 0, time.Local))
	sc.probe.procs = []collector.Proc{{PID: 9, Exe: "javaw.exe"}}
	sc.probe.fg = collector.Foreground{PID: 9, Exe: "javaw.exe", Title: "Minecraft 1.21.4"}
	for day := 6; day <= 8; day++ {
		sc.now = time.Date(2026, 11, day, 18, 0, 0, 0, time.Local)
		sc.tick(1 + 12*day) // day N: N minutes of play
	}
	// Corrupt the other days: if they were read, stderr would report it.
	for _, d := range []string{"2026-11-06", "2026-11-08"} {
		f, err := os.OpenFile(filepath.Join(sc.logDir, d+".jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString("not json\n")
		f.Close()
	}

	for name, args := range map[string][]string{
		"directory":         {"-json", "-date", "2026-11-07", sc.logDir},
		"default (no path)": {"-json", "-date", "2026-11-07"},
	} {
		var out, errb bytes.Buffer
		if err := summaryCmd(args, sc.cfg, sc.logDir, &out, &errb); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if errb.Len() != 0 {
			t.Fatalf("%s: other days' files were read: %s", name, errb.String())
		}
		var days []summary.Day
		if err := json.Unmarshal(out.Bytes(), &days); err != nil {
			t.Fatal(err)
		}
		if len(days) != 1 || days[0].Date != "2026-11-07" || days[0].ActivePlayMin != 7 {
			t.Fatalf("%s: %s", name, out.String())
		}
	}

	var out, errb bytes.Buffer
	if err := summaryCmd([]string{"-date", "2026-11-*", sc.logDir}, sc.cfg, sc.logDir, &out, &errb); err == nil {
		t.Fatal("malformed -date must be rejected")
	}
}
