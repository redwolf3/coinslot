package activity

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/redwolf3/coinslot/core/config"
	"github.com/redwolf3/coinslot/core/internal/synth"
)

const cfgJSON = `{"schema":1,"sample_interval_seconds":5,"idle_threshold_seconds":30,"apps":[
 {"app":"roblox","class":"metered","role":"player","exe":["RobloxPlayerBeta.exe"]},
 {"app":"roblox","class":"metered","role":"launcher","exe":["RobloxPlayerInstaller.exe"]},
 {"app":"minecraft","class":"metered","role":"player","exe":["javaw.exe"],"title_contains":["Minecraft"]},
 {"app":"among-us","class":"metered","role":"player","exe":["Among Us.exe"]},
 {"app":"duolingo","class":"allowed","exe":["msedge.exe"],"title_contains":["Duolingo"]}]}`

func cfg(t *testing.T) *config.Config {
	t.Helper()
	c, err := config.Parse([]byte(cfgJSON))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestClassify(t *testing.T) {
	c := cfg(t)
	roblox := Facts{ConsoleActive: true, FgPID: 9, FgExe: "RobloxPlayerBeta.exe", ProcExes: []string{"RobloxPlayerBeta.exe"}}
	cases := []struct {
		name string
		f    Facts
		want State
	}{
		{"playing", roblox, State{Present: true, Active: "roblox"}},
		{"idle at 31s", func() Facts { f := roblox; f.IdleS = 31; return f }(), State{Open: []string{"roblox"}}},
		{"idle at exactly threshold still counts", func() Facts { f := roblox; f.IdleS = 30; return f }(),
			State{Present: true, Active: "roblox"}},
		{"switched-away session", func() Facts { f := roblox; f.ConsoleActive = false; return f }(), State{Open: []string{"roblox"}}},
		{"launcher in foreground", Facts{ConsoleActive: true, FgPID: 3, FgExe: "RobloxPlayerInstaller.exe",
			ProcExes: []string{"RobloxPlayerInstaller.exe"}}, State{Present: true, Open: []string{"roblox"}}},
		{"game in background, duolingo in front", Facts{ConsoleActive: true, FgPID: 4, FgExe: "msedge.exe", FgTitle: "Duolingo",
			ProcExes: []string{"RobloxPlayerBeta.exe", "msedge.exe"}},
			State{Present: true, Allowed: "duolingo", Open: []string{"roblox"}}},
		{"idle duolingo is not allowed-use", Facts{ConsoleActive: true, FgPID: 4, FgExe: "msedge.exe", FgTitle: "Duolingo", IdleS: 120},
			State{}},
		{"browser on other site", Facts{ConsoleActive: true, FgPID: 4, FgExe: "msedge.exe", FgTitle: "YouTube"}, State{Present: true}},
		{"title-qualified player", Facts{ConsoleActive: true, FgPID: 5, FgExe: "javaw.exe", FgTitle: "Minecraft 1.21",
			ProcExes: []string{"javaw.exe"}}, State{Present: true, Active: "minecraft"}},
		{"title-qualified player idle is open", Facts{ConsoleActive: true, FgPID: 5, FgExe: "javaw.exe", FgTitle: "Minecraft 1.21",
			IdleS: 60, ProcExes: []string{"javaw.exe"}}, State{Open: []string{"minecraft"}}},
		{"two games: one active, one open", Facts{ConsoleActive: true, FgPID: 6, FgExe: "Among Us.exe",
			ProcExes: []string{"Among Us.exe", "RobloxPlayerBeta.exe"}},
			State{Present: true, Active: "among-us", Open: []string{"roblox"}}},
		{"no foreground (lock screen)", Facts{ConsoleActive: true, ProcExes: []string{"RobloxPlayerBeta.exe"}},
			State{Present: true, Open: []string{"roblox"}}},
		{"collector-derived pid 0 ignores exe", Facts{ConsoleActive: true, FgExe: "RobloxPlayerBeta.exe"}, State{Present: true}},
	}
	for _, tc := range cases {
		if got := Classify(c, 30, tc.f); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %+v want %+v", tc.name, got, tc.want)
		}
	}
}

var t0 = time.Date(2026, 11, 6, 18, 0, 0, 0, time.FixedZone("EST", -5*3600))

func replay(t *testing.T, b []byte) []Interval {
	t.Helper()
	evs, _, err := Parse(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return Replay(evs, cfg(t), Options{})
}

func sum(ivs []Interval, pred func(Interval) bool) float64 {
	var s float64
	for _, iv := range ivs {
		if pred(iv) {
			s += iv.Seconds
		}
	}
	return s
}

func TestReplayUsesProcEventsNotDerivedFields(t *testing.T) {
	l := synth.New(`PC\kid1`, "S-1", "r1", t0).Start()
	l.Samples(1, synth.Fg{Exe: "explorer.exe"})
	l.Launch("RobloxPlayerInstaller.exe")
	l.Samples(4, synth.Fg{Exe: "RobloxPlayerInstaller.exe"}) // 20s launcher
	l.Exit("RobloxPlayerInstaller.exe")
	l.Launch("RobloxPlayerBeta.exe")
	l.Samples(12, synth.Fg{Exe: "RobloxPlayerBeta.exe"}) // 60s play
	l.Exit("RobloxPlayerBeta.exe")
	l.Samples(2, synth.Fg{Exe: "explorer.exe"})

	ivs := replay(t, l.Bytes())
	if got := sum(ivs, func(iv Interval) bool { return iv.Active == "roblox" }); got != 60 {
		t.Errorf("active = %v, want 60", got)
	}
	if got := sum(ivs, func(iv Interval) bool { return len(iv.Open) > 0 }); got != 20 {
		t.Errorf("open-not-playing = %v, want 20", got)
	}
	if got := sum(ivs, func(Interval) bool { return true }); got != 18*5 {
		t.Errorf("observed = %v, want 90", got)
	}
	if ivs[0].Seconds != 0 || ivs[0].Day != "2026-11-06" || ivs[0].SID != "S-1" {
		t.Errorf("first interval: %+v", ivs[0])
	}
}

func TestReplayCapsGaps(t *testing.T) {
	l := synth.New("u", "S-1", "r1", t0).Start()
	l.Launch("RobloxPlayerBeta.exe")
	l.Samples(3, synth.Fg{Exe: "RobloxPlayerBeta.exe"}) // 10s credited
	l.Gap(2 * time.Hour)                                // laptop asleep mid-game
	l.Samples(1, synth.Fg{Exe: "RobloxPlayerBeta.exe"}) // capped at 2×5s
	ivs := replay(t, l.Bytes())
	if got := sum(ivs, func(iv Interval) bool { return iv.Active != "" }); got != 20 {
		t.Errorf("active = %v, want 20 (10 + capped 10)", got)
	}
}

func TestReplayRunsKeepSeparateProcState(t *testing.T) {
	a := synth.New("u", "S-1", "runA", t0).Start()
	a.Launch("RobloxPlayerBeta.exe")
	a.Samples(2, synth.Fg{Exe: "explorer.exe"})
	// New run (re-logon): the old run's processes must not leak into it.
	b := synth.New("u", "S-1", "runB", t0.Add(time.Hour)).Start()
	b.Samples(3, synth.Fg{Exe: "explorer.exe"})
	ivs := replay(t, append(a.Bytes(), b.Bytes()...))
	for _, iv := range ivs {
		if iv.Run == "runB" && len(iv.Open) > 0 {
			t.Fatalf("runB inherited runA processes: %+v", iv)
		}
	}
}

func TestParseTolerant(t *testing.T) {
	l := synth.New("u", "S-1", "r", t0).Start()
	l.Samples(2, synth.Fg{Exe: "explorer.exe"})
	l.Raw(`{"v":1,"type":"sample","ts":"2026-11-06T18:00:10.000-05:00","run":"r","seq":9,"dt_s":5,"fg_e`) // truncated
	l.Raw(`not json`)
	l.Raw(`{"v":2,"type":"sample"}`)
	l.Raw(`{"v":1,"type":"stop","reason":"shutdown"}`)
	evs, st, err := Parse(bytes.NewReader(l.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 3 || st.Malformed != 2 || st.Skipped != 2 || st.Lines != 7 {
		t.Fatalf("events=%d stats=%+v", len(evs), st)
	}
}

// Copilot review r4496048688: a failed idle read must not count as recent input.
func TestIdleUnknownIsNotPresent(t *testing.T) {
	c := cfg(t)
	f := Facts{ConsoleActive: true, FgPID: 9, FgExe: "RobloxPlayerBeta.exe", IdleUnknown: true,
		ProcExes: []string{"RobloxPlayerBeta.exe"}}
	if got, want := Classify(c, 30, f), (State{Open: []string{"roblox"}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}

	l := synth.New("u", "S-1", "r1", t0).Start()
	l.Launch("RobloxPlayerBeta.exe")
	l.Samples(3, synth.Fg{Exe: "RobloxPlayerBeta.exe"})                    // 10s active
	l.Samples(4, synth.Fg{Exe: "RobloxPlayerBeta.exe", IdleUnknown: true}) // 20s unknown
	ivs := replay(t, l.Bytes())
	if got := sum(ivs, func(iv Interval) bool { return iv.Active != "" }); got != 10 {
		t.Errorf("active = %v, want 10", got)
	}
	if got := sum(ivs, func(iv Interval) bool { return len(iv.Open) > 0 }); got != 20 {
		t.Errorf("open = %v, want 20", got)
	}
}

// Copilot review r4496048482: a title-identified player (Minecraft Java) stays
// "open" after it leaves the foreground, until its process exits; other javaw.exe
// processes are never counted.
func TestTitledProcessStaysOpenUntilExit(t *testing.T) {
	l := synth.New("u", "S-1", "r1", t0).Start()
	l.Launch("msedge.exe")
	l.Samples(1, synth.Fg{Exe: "explorer.exe"})
	l.Launch("javaw.exe")
	l.Samples(12, synth.Fg{Exe: "javaw.exe", Title: "Minecraft 1.21.4 - Singleplayer"}) // 60s active
	l.Samples(6, synth.Fg{Exe: "msedge.exe", Title: "Duolingo"})                        // 30s, minecraft open
	l.Exit("javaw.exe")
	l.Samples(2, synth.Fg{Exe: "msedge.exe", Title: "Duolingo"}) // 10s, closed
	ivs := replay(t, l.Bytes())
	if got := sum(ivs, func(iv Interval) bool { return iv.Active == "minecraft" }); got != 60 {
		t.Errorf("active = %v, want 60", got)
	}
	if got := sum(ivs, func(iv Interval) bool { return reflect.DeepEqual(iv.Open, []string{"minecraft"}) }); got != 30 {
		t.Errorf("open while in background = %v, want 30", got)
	}

	// A javaw.exe never seen with a Minecraft title stays unclassified.
	l2 := synth.New("u", "S-1", "r2", t0).Start()
	l2.Launch("javaw.exe")
	l2.Samples(4, synth.Fg{Exe: "javaw.exe", Title: "Some IDE"})
	l2.Samples(4, synth.Fg{Exe: "explorer.exe"})
	for _, iv := range replay(t, l2.Bytes()) {
		if len(iv.Open) > 0 || iv.Active != "" {
			t.Fatalf("unrelated javaw classified: %+v", iv)
		}
	}
}
