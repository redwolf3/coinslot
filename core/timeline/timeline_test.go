package timeline

import (
	"strings"
	"testing"
	"time"

	"github.com/redwolf3/coinslot/core/activity"
	"github.com/redwolf3/coinslot/core/config"
	"github.com/redwolf3/coinslot/core/internal/synth"
)

const cfgJSON = `{"schema":1,"sample_interval_seconds":5,"idle_threshold_seconds":30,"apps":[
 {"app":"roblox","class":"metered","role":"player","exe":["RobloxPlayerBeta.exe"]},
 {"app":"roblox","class":"metered","role":"launcher","exe":["RobloxPlayerInstaller.exe"]},
 {"app":"duolingo","class":"allowed","exe":["msedge.exe"],"title_contains":["Duolingo"]}]}`

var zone = time.FixedZone("EST", -5*3600)

func at(day, h, m, s int) time.Time { return time.Date(2026, 11, day, h, m, s, 0, zone) }

func build(t *testing.T, logs ...*synth.Log) []Day {
	t.Helper()
	cfg, err := config.Parse([]byte(cfgJSON))
	if err != nil {
		t.Fatal(err)
	}
	var evs []activity.Event
	for _, l := range logs {
		e, _, err := activity.Parse(strings.NewReader(string(l.Bytes())))
		if err != nil {
			t.Fatal(err)
		}
		evs = append(evs, e...)
	}
	return Build(activity.Replay(evs, cfg, activity.Options{}))
}

func lines(days []Day) []string {
	return strings.Split(strings.TrimRight(Text(days), "\n"), "\n")
}

// The evening scenario from docs/evening-test.md: launcher, play, idle in the game,
// then Duolingo with the game still open.
func TestEveningScenario(t *testing.T) {
	l := synth.New(`PC\kid1`, "S-1", "r1", at(6, 18, 2, 5)).Start()
	l.Launch("RobloxPlayerInstaller.exe")
	l.Samples(1+37, synth.Fg{Exe: "RobloxPlayerInstaller.exe"}) // 18:02:05-18:05:10
	l.Exit("RobloxPlayerInstaller.exe")
	l.Launch("RobloxPlayerBeta.exe")
	l.Samples(318, synth.Fg{Exe: "RobloxPlayerBeta.exe"})           // -18:31:40
	l.Samples(52, synth.Fg{Exe: "RobloxPlayerBeta.exe", IdleS: 60}) // -18:36:00
	l.Samples(168, synth.Fg{Exe: "msedge.exe", Title: "Duolingo"})  // -18:50:00

	days := build(t, l)
	want := []string{
		`2026-11-06  PC\kid1  (S-1)`,
		`  18:02:05–18:05:10  roblox       open, not playing (RobloxPlayerInstaller.exe in front)`,
		`  18:05:10–18:31:40  roblox       ACTIVE`,
		`  18:31:40–18:36:00  roblox       open, not playing (idle)`,
		`  18:36:00–18:50:00  duolingo     allowed   [roblox open]`,
	}
	got := lines(days)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// Consecutive intervals of one state merge, even when the sample's idle seconds
// differ, and a change of foreground exe splits the segment.
func TestMergeAndSplit(t *testing.T) {
	l := synth.New("u", "S-1", "r1", at(6, 18, 0, 0)).Start()
	l.Launch("RobloxPlayerBeta.exe")
	l.Samples(1, synth.Fg{Exe: "RobloxPlayerBeta.exe", IdleS: 1})
	l.Samples(6, synth.Fg{Exe: "RobloxPlayerBeta.exe", IdleS: 7}) // 30s
	l.Samples(6, synth.Fg{Exe: "RobloxPlayerBeta.exe", IdleS: 2}) // 30s, same state
	l.Samples(2, synth.Fg{Exe: "explorer.exe"})                   // 10s, roblox in background
	days := build(t, l)
	segs := days[0].Segments
	if len(segs) != 2 {
		t.Fatalf("segments = %+v", segs)
	}
	if d := segs[0].End.Sub(segs[0].Start); d != time.Minute || segs[0].Active != "roblox" {
		t.Fatalf("first segment %+v (%v)", segs[0], d)
	}
	if segs[1].Active != "" || segs[1].FgExe != "explorer.exe" || segs[1].Open[0] != "roblox" {
		t.Fatalf("second segment %+v", segs[1])
	}
}

// Sleep: the credit is capped, so the uncovered time is shown as a gap and the
// state before and after it stays in separate segments.
func TestCappedCreditShowsGap(t *testing.T) {
	l := synth.New("u", "S-1", "r1", at(6, 18, 0, 0)).Start()
	l.Launch("RobloxPlayerBeta.exe")
	l.Samples(13, synth.Fg{Exe: "RobloxPlayerBeta.exe"}) // 18:00:00-18:01:00
	l.Gap(10 * time.Minute)
	l.Samples(13, synth.Fg{Exe: "RobloxPlayerBeta.exe"}) // first sample after the gap credits the 10s cap
	got := lines(build(t, l))
	want := []string{
		"2026-11-06  u  (S-1)",
		"  18:00:00–18:01:00  roblox       ACTIVE",
		"  18:01:00–18:10:55  -            (no data, 9m55s)",
		"  18:10:55–18:12:05  roblox       ACTIVE",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s", strings.Join(got, "\n"))
	}
}

// A new run (collector restart) starts a new segment even when the state is the same,
// and the time between the runs is shown.
func TestNewRunStartsSegment(t *testing.T) {
	a := synth.New("u", "S-1", "r1", at(6, 18, 0, 0)).Start()
	a.Launch("RobloxPlayerBeta.exe")
	a.Samples(13, synth.Fg{Exe: "RobloxPlayerBeta.exe"}) // 18:00:00-18:01:00
	b := synth.New("u", "S-1", "r2", at(6, 18, 2, 0)).Start()
	b.Present("RobloxPlayerBeta.exe")
	b.Samples(13, synth.Fg{Exe: "RobloxPlayerBeta.exe"}) // 18:02:00-18:03:00
	got := lines(build(t, a, b))
	want := []string{
		"2026-11-06  u  (S-1)",
		"  18:00:00–18:01:00  roblox       ACTIVE",
		"  18:01:00–18:02:00  -            (no data, 1m00s)",
		"  18:02:00–18:03:00  roblox       ACTIVE",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s", strings.Join(got, "\n"))
	}
}

// Back-to-back runs with the same state (restart with no downtime) are still two
// segments, with no gap between them.
func TestNewRunWithoutGapIsNotMerged(t *testing.T) {
	a := synth.New("u", "S-1", "r1", at(6, 18, 0, 0)).Start()
	a.Launch("RobloxPlayerBeta.exe")
	a.Samples(13, synth.Fg{Exe: "RobloxPlayerBeta.exe"}) // 18:00:00-18:01:00
	b := synth.New("u", "S-1", "r2", at(6, 18, 1, 0)).Start()
	b.Present("RobloxPlayerBeta.exe")
	b.Samples(13, synth.Fg{Exe: "RobloxPlayerBeta.exe"}) // 18:01:00-18:02:00
	segs := build(t, a, b)[0].Segments
	if len(segs) != 2 || segs[0].Gap || segs[1].Gap || !segs[0].End.Equal(segs[1].Start) {
		t.Fatalf("segments = %+v", segs)
	}
}

// Each child and each local date gets its own timeline; an interval that crosses
// midnight is split between the two days.
func TestPerChildAndPerDay(t *testing.T) {
	a := synth.New(`PC\kid1`, "S-1", "r1", at(6, 23, 59, 0)).Start()
	a.Launch("RobloxPlayerBeta.exe")
	a.Samples(1+36, synth.Fg{Exe: "RobloxPlayerBeta.exe"}) // 23:59:00-00:02:00
	b := synth.New(`PC\kid2`, "S-2", "r2", at(6, 23, 59, 0)).Start()
	b.Samples(1+12, synth.Fg{Exe: "msedge.exe", Title: "YouTube"}) // 23:59:00-00:00:00
	days := build(t, a, b)
	var keys []string
	for _, d := range days {
		keys = append(keys, d.Date+"/"+d.SID)
	}
	if strings.Join(keys, " ") != "2026-11-06/S-1 2026-11-06/S-2 2026-11-07/S-1" {
		t.Fatalf("days = %v", keys)
	}
	if s := days[0].Segments; len(s) != 1 || !s[0].End.Equal(at(7, 0, 0, 0)) {
		t.Fatalf("kid1 day 1: %+v", s)
	}
	if s := days[2].Segments; len(s) != 1 || !s[0].Start.Equal(at(7, 0, 0, 0)) || !s[0].End.Equal(at(7, 0, 2, 0)) {
		t.Fatalf("kid1 day 2: %+v", s)
	}
	if s := days[1].Segments; len(s) != 1 || s[0].Present != true || s[0].Active != "" || len(s[0].Open) != 0 {
		t.Fatalf("kid2: %+v", s)
	}
	if got := lines(days[1:2])[1]; got != "  23:59:00–00:00:00  -            msedge.exe in front" {
		t.Fatalf("kid2 line = %q", got)
	}
}

func TestIdleWithNothingOpen(t *testing.T) {
	l := synth.New("u", "S-1", "r1", at(6, 18, 0, 0)).Start()
	l.Samples(3, synth.Fg{Exe: "explorer.exe", IdleS: 300})
	if got := lines(build(t, l))[1]; got != "  18:00:00–18:00:10  -            idle" {
		t.Fatalf("line = %q", got)
	}
}
