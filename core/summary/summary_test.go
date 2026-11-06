package summary

import (
	"bytes"
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
 {"app":"among-us","class":"metered","role":"player","exe":["Among Us.exe"]},
 {"app":"duolingo","class":"allowed","exe":["msedge.exe"],"title_contains":["Duolingo"]}]}`

var tz = time.FixedZone("EST", -5*3600)

func run(t *testing.T, logs ...[]byte) []Day {
	t.Helper()
	c, err := config.Parse([]byte(cfgJSON))
	if err != nil {
		t.Fatal(err)
	}
	evs, _, err := activity.Parse(bytes.NewReader(bytes.Join(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return Summarize(activity.Replay(evs, c, activity.Options{}))
}

const perMin = 12 // samples per minute at 5s

// An evening for kid1: Duolingo, Roblox update, play with an idle break,
// Roblox left open behind Duolingo, then Among Us.
func kid1Evening() []byte {
	l := synth.New(`PC\kid1`, "S-1-5-21-1001", "k1", time.Date(2026, 11, 6, 18, 0, 0, 0, tz)).Start()
	l.Launch("msedge.exe")
	l.Samples(1, synth.Fg{Exe: "explorer.exe"})                          // 18:00:00 (credits 0)
	l.Samples(10*perMin, synth.Fg{Exe: "msedge.exe", Title: "Duolingo"}) // 10 min allowed
	l.Launch("RobloxPlayerInstaller.exe")
	l.Samples(3*perMin, synth.Fg{Exe: "RobloxPlayerInstaller.exe"}) // 3 min updating
	l.Exit("RobloxPlayerInstaller.exe")
	l.Launch("RobloxPlayerBeta.exe")
	l.Samples(20*perMin, synth.Fg{Exe: "RobloxPlayerBeta.exe"})           // 20 min play
	l.Samples(5*perMin, synth.Fg{Exe: "RobloxPlayerBeta.exe", IdleS: 45}) // 5 min AFK
	l.Samples(10*perMin, synth.Fg{Exe: "RobloxPlayerBeta.exe"})           // 10 min play
	l.Samples(4*perMin, synth.Fg{Exe: "msedge.exe", Title: "Duolingo"})   // 4 min allowed, roblox open
	l.Exit("RobloxPlayerBeta.exe")
	l.Launch("Among Us.exe")
	l.Samples(15*perMin, synth.Fg{Exe: "Among Us.exe"}) // 15 min play
	l.Exit("Among Us.exe")
	l.Samples(2*perMin, synth.Fg{Exe: "explorer.exe", IdleS: 300}) // 2 min nothing
	return l.Bytes()
}

func TestEvening(t *testing.T) {
	days := run(t, kid1Evening())
	if len(days) != 1 {
		t.Fatalf("days = %d", len(days))
	}
	d := days[0]
	check := func(name string, got, want float64) {
		t.Helper()
		if got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	check("observed", d.ObservedMin, 69)
	check("active play", d.ActivePlayMin, 45)
	check("allowed", d.AllowedMin, 14)
	check("open not playing", d.OpenNotPlayingMin, 3+5+4)
	check("present", d.PresentMin, 69-5-2)
	if len(d.Games) != 2 || d.Games[0].App != "among-us" || d.Games[1].App != "roblox" {
		t.Fatalf("games: %+v", d.Games)
	}
	am, rb := d.Games[0], d.Games[1]
	check("among-us active", am.ActiveMin, 15)
	check("roblox active", rb.ActiveMin, 30)
	check("roblox open", rb.OpenNotPlayingMin, 12)
	if rb.FirstActive != "18:13" || rb.LastActive != "18:48" {
		t.Errorf("roblox span %s–%s", rb.FirstActive, rb.LastActive)
	}
	if len(d.Allowed) != 1 || d.Allowed[0].ActiveMin != 14 {
		t.Errorf("allowed: %+v", d.Allowed)
	}
	txt := Text(days)
	for _, want := range []string{`PC\kid1`, "ACTIVE PLAY", "45.0m", "roblox", "30.0m", "18:13–18:48", "duolingo"} {
		if !strings.Contains(txt, want) {
			t.Errorf("text missing %q:\n%s", want, txt)
		}
	}
}

func TestTwoKidsSeparated(t *testing.T) {
	k2 := synth.New(`PC\kid2`, "S-1-5-21-1002", "k2", time.Date(2026, 11, 6, 19, 0, 0, 0, tz)).Start()
	k2.Launch("RobloxPlayerBeta.exe")
	k2.Samples(1+7*perMin, synth.Fg{Exe: "RobloxPlayerBeta.exe"})
	days := run(t, kid1Evening(), k2.Bytes())
	if len(days) != 2 || days[0].User != `PC\kid1` || days[1].User != `PC\kid2` {
		t.Fatalf("days: %+v", days)
	}
	if days[1].ActivePlayMin != 7 || days[0].ActivePlayMin != 45 {
		t.Errorf("kid2=%v kid1=%v", days[1].ActivePlayMin, days[0].ActivePlayMin)
	}
}

func TestMidnightSplitsDays(t *testing.T) {
	l := synth.New("u", "S-1", "r", time.Date(2026, 11, 6, 23, 55, 0, 0, tz)).Start()
	l.Launch("RobloxPlayerBeta.exe")
	l.Samples(1+10*perMin, synth.Fg{Exe: "RobloxPlayerBeta.exe"}) // 23:55 -> 00:05
	days := run(t, l.Bytes())
	if len(days) != 2 || days[0].Date != "2026-11-06" || days[1].Date != "2026-11-07" {
		t.Fatalf("days: %+v", days)
	}
	if days[0].ActivePlayMin != 5 || days[1].ActivePlayMin != 5 {
		t.Errorf("split %v / %v", days[0].ActivePlayMin, days[1].ActivePlayMin)
	}
}

func TestHM(t *testing.T) {
	for in, want := range map[float64]string{0: "0.0m", 12.34: "12.3m", 60: "1h00m", 125.4: "2h05m"} {
		if got := hm(in); got != want {
			t.Errorf("hm(%v) = %q, want %q", in, got, want)
		}
	}
}
