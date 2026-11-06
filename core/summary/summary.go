// Package summary aggregates classified intervals into a daily summary per child.
// It is pure and platform-independent.
package summary

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/redwolf3/coinslot/core/activity"
)

// Game is one metered app's time on one day.
type Game struct {
	App string `json:"app"`
	// ActiveMin: player in foreground with recent input.
	ActiveMin float64 `json:"active_min"`
	// OpenNotPlayingMin: player or launcher open, but not actively played
	// (background, idle, launcher/update in front).
	OpenNotPlayingMin float64 `json:"open_not_playing_min"`
	FirstActive       string  `json:"first_active,omitempty"` // local HH:MM
	LastActive        string  `json:"last_active,omitempty"`
}

type App struct {
	App       string  `json:"app"`
	ActiveMin float64 `json:"active_min"`
}

// Day is one child's summary for one local date.
type Day struct {
	Date string `json:"date"`
	User string `json:"user"`
	SID  string `json:"sid"`
	Runs int    `json:"runs"`
	// ObservedMin: time the collector was sampling (credited), whatever was happening.
	ObservedMin float64 `json:"observed_min"`
	// PresentMin: session active with input within the idle threshold.
	PresentMin float64 `json:"present_min"`
	// ActivePlayMin: sum of Games[].ActiveMin (at most one game is active at a time).
	ActivePlayMin float64 `json:"active_play_min"`
	// OpenNotPlayingMin: any metered game open while none was actively played.
	// Not the sum of per-game values: two games open at once count once here.
	OpenNotPlayingMin float64 `json:"open_not_playing_min"`
	AllowedMin        float64 `json:"allowed_min"`
	Games             []Game  `json:"games"`
	Allowed           []App   `json:"allowed"`
}

type acc struct {
	day                         Day
	runs                        map[string]bool
	obs, present, active, open  float64
	allowed                     float64
	gameActive, gameOpen, allow map[string]float64
	first, last                 map[string]time.Time
}

// Summarize groups intervals by (local date, SID). Output is sorted by date, then user.
func Summarize(ivs []activity.Interval) []Day {
	accs := map[[2]string]*acc{}
	for _, iv := range ivs {
		key := [2]string{iv.Day, iv.SID}
		a := accs[key]
		if a == nil {
			a = &acc{day: Day{Date: iv.Day, User: iv.User, SID: iv.SID}, runs: map[string]bool{},
				gameActive: map[string]float64{}, gameOpen: map[string]float64{}, allow: map[string]float64{},
				first: map[string]time.Time{}, last: map[string]time.Time{}}
			accs[key] = a
		}
		a.runs[iv.Run] = true
		s := iv.Seconds
		a.obs += s
		if iv.Present {
			a.present += s
		}
		if iv.Active != "" {
			a.active += s
			a.gameActive[iv.Active] += s
			if s > 0 {
				start := iv.End.Add(-time.Duration(s * float64(time.Second)))
				if f, ok := a.first[iv.Active]; !ok || start.Before(f) {
					a.first[iv.Active] = start
				}
				if iv.End.After(a.last[iv.Active]) {
					a.last[iv.Active] = iv.End
				}
			}
		} else if len(iv.Open) > 0 {
			a.open += s
		}
		for _, app := range iv.Open {
			a.gameOpen[app] += s
		}
		if iv.Allowed != "" {
			a.allowed += s
			a.allow[iv.Allowed] += s
		}
	}

	out := make([]Day, 0, len(accs))
	for _, a := range accs {
		d := a.day
		d.Runs = len(a.runs)
		d.ObservedMin, d.PresentMin = mins(a.obs), mins(a.present)
		d.ActivePlayMin, d.OpenNotPlayingMin, d.AllowedMin = mins(a.active), mins(a.open), mins(a.allowed)
		apps := map[string]bool{}
		for app := range a.gameActive {
			apps[app] = true
		}
		for app := range a.gameOpen {
			apps[app] = true
		}
		for app := range apps {
			g := Game{App: app, ActiveMin: mins(a.gameActive[app]), OpenNotPlayingMin: mins(a.gameOpen[app])}
			if f, ok := a.first[app]; ok {
				g.FirstActive, g.LastActive = f.Format("15:04"), a.last[app].Format("15:04")
			}
			d.Games = append(d.Games, g)
		}
		sort.Slice(d.Games, func(i, j int) bool { return d.Games[i].App < d.Games[j].App })
		for app, s := range a.allow {
			d.Allowed = append(d.Allowed, App{App: app, ActiveMin: mins(s)})
		}
		sort.Slice(d.Allowed, func(i, j int) bool { return d.Allowed[i].App < d.Allowed[j].App })
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].User < out[j].User
	})
	return out
}

func mins(s float64) float64 { return math.Round(s/60*100) / 100 }

// Text renders a human-readable summary for comparing against what you saw.
func Text(days []Day) string {
	var b strings.Builder
	for i, d := range days {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s  %s  (%s)\n", d.Date, d.User, d.SID)
		fmt.Fprintf(&b, "  collector observed  %8s  (%d run(s)); present %s\n", hm(d.ObservedMin), d.Runs, hm(d.PresentMin))
		fmt.Fprintf(&b, "  ACTIVE PLAY         %8s\n", hm(d.ActivePlayMin))
		for _, g := range d.Games {
			span := ""
			if g.FirstActive != "" {
				span = fmt.Sprintf("  %s–%s", g.FirstActive, g.LastActive)
			}
			fmt.Fprintf(&b, "    %-16s  %8s  open, not playing %s%s\n", g.App, hm(g.ActiveMin), hm(g.OpenNotPlayingMin), span)
		}
		fmt.Fprintf(&b, "  open, not playing   %8s\n", hm(d.OpenNotPlayingMin))
		fmt.Fprintf(&b, "  allowed apps        %8s\n", hm(d.AllowedMin))
		for _, a := range d.Allowed {
			fmt.Fprintf(&b, "    %-16s  %8s\n", a.App, hm(a.ActiveMin))
		}
	}
	return b.String()
}

// hm formats minutes as "1h05m" or "12.5m".
func hm(m float64) string {
	if m < 60 {
		return fmt.Sprintf("%.1fm", m)
	}
	total := int(math.Round(m))
	return fmt.Sprintf("%dh%02dm", total/60, total%60)
}
