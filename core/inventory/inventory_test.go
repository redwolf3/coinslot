package inventory

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/redwolf3/coinslot/core/activity"
	"github.com/redwolf3/coinslot/core/config"
	"github.com/redwolf3/coinslot/core/internal/synth"
)

// The Sunday scenario: the config guesses wrong names (the real player is
// "RobloxPlayerNew.exe"), so the inventory must surface the real exe from the log.
const wrongCfg = `{"schema":1,"apps":[
 {"app":"roblox","class":"metered","role":"player","exe":["RobloxPlayerBeta.exe"]},
 {"app":"duolingo","class":"allowed","exe":["msedge.exe"],"title_contains":["Duolingo"]}]}`

var t0 = time.Date(2026, 11, 8, 18, 0, 0, 0, time.FixedZone("EST", -5*3600))

func build(t *testing.T, cfgJSON string, logs ...[]byte) Report {
	t.Helper()
	cfg, err := config.Parse([]byte(cfgJSON))
	if err != nil {
		t.Fatal(err)
	}
	evs, _, err := activity.Parse(bytes.NewReader(bytes.Join(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return Build(evs, cfg)
}

func evening() []byte {
	l := synth.New(`PC\kid1`, "S-1", "r1", t0).Start()
	l.Present("explorer.exe")
	l.Present("svchost.exe") // system noise: running at start, never in front
	l.Samples(1, synth.Fg{Exe: "explorer.exe"})
	l.Launch("msedge.exe")
	l.Samples(12*5, synth.Fg{Exe: "msedge.exe", Title: "Duolingo - Lesson"}) // 5 min
	l.Samples(12*2, synth.Fg{Exe: "msedge.exe", Title: "YouTube"})           // 2 min
	l.Launch("RobloxInstallerX.exe")
	l.Samples(12*1, synth.Fg{Exe: "RobloxInstallerX.exe", Title: "Roblox"}) // 1 min update
	l.Exit("RobloxInstallerX.exe")
	l.Launch("RobloxPlayerNew.exe")
	l.Samples(12*10, synth.Fg{Exe: "RobloxPlayerNew.exe", Title: "Roblox"})           // 10 min play
	l.Samples(12*2, synth.Fg{Exe: "RobloxPlayerNew.exe", Title: "Roblox", IdleS: 60}) // 2 min idle
	l.Exit("RobloxPlayerNew.exe")
	l.Launch("backgroundTaskHost.exe") // started, never in front
	l.Samples(12*1, synth.Fg{})        // 1 min lock screen
	return l.Bytes()
}

func TestForeground(t *testing.T) {
	r := build(t, wrongCfg, evening())
	if r.Users[0] != `PC\kid1` || r.From == "" || r.To == "" {
		t.Fatalf("header: %+v", r)
	}
	byExe := map[string]Foreground{}
	for _, f := range r.Foreground {
		byExe[f.Exe] = f
	}
	rb := byExe["RobloxPlayerNew.exe"]
	if rb.Seconds != 12*60 || rb.PresentSeconds != 10*60 || rb.Match != "-" {
		t.Errorf("roblox player: %+v", rb)
	}
	if len(rb.Paths) != 1 || rb.Paths[0] != synth.FakePath("RobloxPlayerNew.exe") {
		t.Errorf("roblox paths: %v", rb.Paths)
	}
	if r.Foreground[0].Exe != "RobloxPlayerNew.exe" {
		t.Errorf("not sorted by time: first is %q", r.Foreground[0].Exe)
	}
	edge := byExe["msedge.exe"]
	if edge.Seconds != 7*60 || len(edge.Titles) != 2 || edge.Titles[0].Title != "Duolingo - Lesson" ||
		edge.Titles[0].Seconds != 5*60 || edge.Match != "duolingo/allowed/player" {
		t.Errorf("edge: %+v", edge)
	}
	if none := byExe[""]; none.Seconds != 60 || none.Match != "-" {
		t.Errorf("no-window: %+v", none)
	}
}

func TestProcesses(t *testing.T) {
	r := build(t, wrongCfg, evening())
	byExe := map[string]Process{}
	for _, p := range r.Processes {
		byExe[p.Exe] = p
	}
	if _, ok := byExe["svchost.exe"]; ok {
		t.Error("system process present at start and never in front should be hidden")
	}
	if p, ok := byExe["explorer.exe"]; !ok || p.Starts != 0 || !p.EverForeground {
		t.Errorf("explorer (present, in front) should be listed: %+v", p)
	}
	inst := byExe["RobloxInstallerX.exe"]
	if inst.Starts != 1 || inst.RunningSeconds != 60 || !inst.EverForeground || inst.FirstSeen != "18:07:00" {
		t.Errorf("installer: %+v", inst)
	}
	pl := byExe["RobloxPlayerNew.exe"]
	if pl.RunningSeconds != 12*60 || pl.Match != "-" {
		t.Errorf("player: %+v", pl)
	}
	bg := byExe["backgroundTaskHost.exe"]
	if bg.Starts != 1 || bg.EverForeground || bg.RunningSeconds != 60 {
		t.Errorf("open process should run until the end of the log: %+v", bg)
	}
}

func TestAfterConfigFixMatches(t *testing.T) {
	fixed := strings.Replace(wrongCfg, "RobloxPlayerBeta.exe", "RobloxPlayerNew.exe", 1)
	r := build(t, fixed, evening())
	for _, f := range r.Foreground {
		if f.Exe == "RobloxPlayerNew.exe" && f.Match != "roblox/metered/player" {
			t.Fatalf("match after fix: %q", f.Match)
		}
	}
}

func TestText(t *testing.T) {
	r := build(t, wrongCfg, evening())
	txt := Text(r, 2)
	for _, want := range []string{"FOREGROUND", "RobloxPlayerNew.exe", "12m00s", "with input", "10m00s",
		`"Duolingo - Lesson"`, "PROCESSES", "more (use -top 0)", synth.FakePath("RobloxPlayerNew.exe")} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
}

func TestDurAndClip(t *testing.T) {
	if dur(3725) != "1h02m" || dur(65.4) != "1m05s" {
		t.Errorf("dur: %s %s", dur(3725), dur(65.4))
	}
	if got := clip("abcdef", 4); got != "abc…" {
		t.Errorf("clip: %q", got)
	}
}

// Copilot review r4496048547: From/To must not depend on input order.
func TestRangeIndependentOfOrder(t *testing.T) {
	late := synth.New(`PC\kid1`, "S-1", "a", t0).Start()
	late.Samples(1+12*60, synth.Fg{Exe: "explorer.exe"}) // 18:00-19:00
	early := synth.New(`PC\kid2`, "S-2", "b", t0.Add(-2*time.Hour)).Start()
	early.Samples(1+12*10, synth.Fg{Exe: "explorer.exe"}) // 16:00-16:10
	r := build(t, wrongCfg, late.Bytes(), early.Bytes())
	if r.From != "2026-11-08T16:00:00.000-05:00" || r.To != "2026-11-08T19:00:00.000-05:00" {
		t.Fatalf("range %s .. %s", r.From, r.To)
	}
}
