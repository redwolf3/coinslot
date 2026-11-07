// Package timeline merges classified intervals into segments of identical state, per
// child and local date, so a log can be read side by side with hand-written notes.
// It is pure and platform-independent.
package timeline

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/redwolf3/coinslot/core/activity"
)

// gapTolerance is the largest mismatch between one interval's end and the next one's
// start that is still treated as contiguous. Wall-clock timestamps and the monotonic
// dt_s differ by milliseconds; anything larger is uncovered time.
const gapTolerance = time.Second

// Segment is a stretch of time in one state. A Gap segment is time the collector did
// not cover (capped credit after sleep or a stall, or between runs); it has no state.
type Segment struct {
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Gap     bool      `json:"gap,omitempty"`
	Present bool      `json:"present"`
	Active  string    `json:"active,omitempty"`
	Allowed string    `json:"allowed,omitempty"`
	Open    []string  `json:"open,omitempty"`
	FgExe   string    `json:"fg_exe,omitempty"`

	run string
}

// Day is one child's timeline for one local date.
type Day struct {
	Date     string    `json:"date"`
	User     string    `json:"user"`
	SID      string    `json:"sid"`
	Segments []Segment `json:"segments"`
}

// Build groups intervals by (local date, SID) and merges consecutive intervals of the
// same run and state into segments. A segment starts at the interval end minus its
// credited seconds. Intervals that credit nothing (the first sample of a run) are
// dropped. A new run, or time not covered by any interval, ends the segment; uncovered
// time is returned as an explicit Gap segment. Output is sorted by date, then user.
func Build(ivs []activity.Interval) []Day {
	type group struct {
		day Day
		ivs []activity.Interval
	}
	groups := map[[2]string]*group{}
	for _, iv := range ivs {
		if iv.Seconds <= 0 {
			continue
		}
		key := [2]string{iv.Day, iv.SID}
		g := groups[key]
		if g == nil {
			g = &group{day: Day{Date: iv.Day, User: iv.User, SID: iv.SID}}
			groups[key] = g
		}
		g.ivs = append(g.ivs, iv)
	}

	out := make([]Day, 0, len(groups))
	for _, g := range groups {
		sort.SliceStable(g.ivs, func(i, j int) bool { return start(g.ivs[i]).Before(start(g.ivs[j])) })
		d := g.day
		for _, iv := range g.ivs {
			cur := Segment{Start: start(iv), End: iv.End, Present: iv.Present, Active: iv.Active,
				Allowed: iv.Allowed, Open: iv.Open, FgExe: iv.FgExe, run: iv.Run}
			if n := len(d.Segments); n > 0 {
				prev := &d.Segments[n-1]
				if cur.Start.Sub(prev.End) > gapTolerance {
					d.Segments = append(d.Segments, Segment{Start: prev.End, End: cur.Start, Gap: true})
				} else if prev.run == cur.run && sameState(*prev, cur) {
					if cur.End.After(prev.End) {
						prev.End = cur.End
					}
					continue
				}
			}
			d.Segments = append(d.Segments, cur)
		}
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

func start(iv activity.Interval) time.Time {
	return iv.End.Add(-time.Duration(iv.Seconds * float64(time.Second)))
}

func sameState(a, b Segment) bool {
	if a.Present != b.Present || a.Active != b.Active || a.Allowed != b.Allowed || a.FgExe != b.FgExe ||
		len(a.Open) != len(b.Open) {
		return false
	}
	for i := range a.Open {
		if a.Open[i] != b.Open[i] {
			return false
		}
	}
	return true
}

// Text renders the timeline, one line per segment, for comparing against notes.
func Text(days []Day) string {
	var b strings.Builder
	for i, d := range days {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s  %s  (%s)\n", d.Date, d.User, d.SID)
		for _, s := range d.Segments {
			fmt.Fprintf(&b, "  %s–%s  %s\n", clock(s.Start), clock(s.End), describe(s))
		}
	}
	return b.String()
}

// clock formats to the nearest second so that the end of one segment and the start of
// the next read identically.
func clock(t time.Time) string { return t.Round(time.Second).Format("15:04:05") }

func describe(s Segment) string {
	if s.Gap {
		return fmt.Sprintf("%-12s (no data, %s)", "-", span(s.End.Sub(s.Start)))
	}
	others := ""
	if len(s.Open) > 0 && (s.Active != "" || s.Allowed != "") {
		others = fmt.Sprintf("   [%s open]", strings.Join(s.Open, ", "))
	}
	switch {
	case s.Active != "":
		return fmt.Sprintf("%-12s ACTIVE%s", s.Active, others)
	case s.Allowed != "":
		return fmt.Sprintf("%-12s allowed%s", s.Allowed, others)
	case len(s.Open) > 0:
		return fmt.Sprintf("%-12s open, not playing (%s)", strings.Join(s.Open, ", "), where(s))
	case s.Present:
		return fmt.Sprintf("%-12s %s", "-", where(s))
	}
	return fmt.Sprintf("%-12s idle", "-")
}

// where says what the child was looking at, or that they were idle.
func where(s Segment) string {
	switch {
	case !s.Present:
		return "idle"
	case s.FgExe == "":
		return "no foreground window"
	}
	return s.FgExe + " in front"
}

// span formats a duration as "45s", "5m30s" or "1h02m03s".
func span(d time.Duration) string {
	sec := int(d.Round(time.Second) / time.Second)
	switch {
	case sec >= 3600:
		return fmt.Sprintf("%dh%02dm%02ds", sec/3600, sec%3600/60, sec%60)
	case sec >= 60:
		return fmt.Sprintf("%dm%02ds", sec/60, sec%60)
	}
	return fmt.Sprintf("%ds", sec)
}
