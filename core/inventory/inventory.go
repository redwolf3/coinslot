// Package inventory reports which programs were in the foreground and which
// processes started, from collector logs. It exists so the process-name
// inventory can be done after collecting, from the logs, instead of by hand
// before. It is pure and platform-independent.
package inventory

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/redwolf3/coinslot/core/activity"
	"github.com/redwolf3/coinslot/core/config"
	"github.com/redwolf3/coinslot/core/record"
)

const maxTitles = 5

type Title struct {
	Title   string  `json:"title"`
	Seconds float64 `json:"seconds"`
}

// Foreground is one exe's foreground time.
type Foreground struct {
	Exe            string   `json:"exe"` // "" = no foreground window (lock screen etc.)
	Paths          []string `json:"paths,omitempty"`
	Seconds        float64  `json:"seconds"`          // credited foreground time
	PresentSeconds float64  `json:"present_seconds"`  // ... of which with recent input
	Titles         []Title  `json:"titles,omitempty"` // top titles by time
	Match          string   `json:"match"`            // app/class/role under the current config, or "-"
}

// Process is one exe's process lifetime summed over all its instances.
type Process struct {
	Exe            string   `json:"exe"`
	Paths          []string `json:"paths,omitempty"`
	Starts         int      `json:"starts"` // instances started while the collector ran
	RunningSeconds float64  `json:"running_seconds"`
	FirstSeen      string   `json:"first_seen"`
	EverForeground bool     `json:"ever_foreground"`
	Match          string   `json:"match"`
}

type Report struct {
	From       string       `json:"from"`
	To         string       `json:"to"`
	Users      []string     `json:"users"`
	Foreground []Foreground `json:"foreground"`
	// Processes started while the collector ran, or ever in the foreground.
	// Processes already running at collector start and never in front (system
	// noise) are left out.
	Processes []Process `json:"processes"`
}

// Build makes the report. cfg is used for the Match column and the idle threshold.
func Build(evs []activity.Event, cfg *config.Config) Report {
	var rep Report
	ivs := activity.Replay(evs, cfg, activity.Options{})

	users := map[string]bool{}
	type fgAcc struct {
		f      Foreground
		paths  map[string]bool
		titles map[string]float64
	}
	fgs := map[string]*fgAcc{}
	for _, iv := range ivs {
		users[iv.User] = true
		key := strings.ToLower(iv.FgExe)
		a := fgs[key]
		if a == nil {
			a = &fgAcc{f: Foreground{Exe: iv.FgExe}, paths: map[string]bool{}, titles: map[string]float64{}}
			fgs[key] = a
		}
		a.f.Seconds += iv.Seconds
		if iv.Present {
			a.f.PresentSeconds += iv.Seconds
		}
		if iv.FgPath != "" {
			a.paths[iv.FgPath] = true
		}
		a.titles[iv.FgTitle] += iv.Seconds
	}
	for key, a := range fgs {
		f := a.f
		f.Paths = sortedKeys(a.paths)
		for t, s := range a.titles {
			f.Titles = append(f.Titles, Title{Title: t, Seconds: s})
		}
		sort.Slice(f.Titles, func(i, j int) bool {
			if f.Titles[i].Seconds != f.Titles[j].Seconds {
				return f.Titles[i].Seconds > f.Titles[j].Seconds
			}
			return f.Titles[i].Title < f.Titles[j].Title
		})
		if len(f.Titles) > maxTitles {
			f.Titles = f.Titles[:maxTitles]
		}
		f.Match = "-"
		if key != "" && len(f.Titles) > 0 {
			f.Match = describe(cfg.Match(f.Exe, f.Titles[0].Title))
		}
		rep.Foreground = append(rep.Foreground, f)
	}
	sort.Slice(rep.Foreground, func(i, j int) bool {
		if rep.Foreground[i].Seconds != rep.Foreground[j].Seconds {
			return rep.Foreground[i].Seconds > rep.Foreground[j].Seconds
		}
		return rep.Foreground[i].Exe < rep.Foreground[j].Exe
	})

	inFront := map[string]bool{}
	for key := range fgs {
		inFront[key] = true
	}
	rep.Processes = processes(evs, cfg, inFront)
	for u := range users {
		rep.Users = append(rep.Users, u)
	}
	sort.Strings(rep.Users)
	if len(ivs) > 0 {
		rep.From = ivs[0].End.Format(record.TimeLayout)
		rep.To = ivs[len(ivs)-1].End.Format(record.TimeLayout)
	}
	return rep
}

type live struct {
	exe   string
	start time.Time
}

func processes(evs []activity.Event, cfg *config.Config, inFront map[string]bool) []Process {
	type acc struct {
		p       Process
		paths   map[string]bool
		first   time.Time
		started bool
	}
	accs := map[string]*acc{}
	get := func(exe string) *acc {
		key := strings.ToLower(exe)
		a := accs[key]
		if a == nil {
			a = &acc{p: Process{Exe: exe}, paths: map[string]bool{}}
			accs[key] = a
		}
		return a
	}
	running := map[string]map[uint32]live{} // run -> pid -> process
	last := map[string]time.Time{}          // run -> last event time
	closeOne := func(l live, end time.Time) {
		if end.After(l.start) {
			get(l.exe).p.RunningSeconds += end.Sub(l.start).Seconds()
		}
	}

	for _, ev := range evs {
		ts, err := time.Parse(record.TimeLayout, ev.Header.TS)
		if err != nil {
			continue
		}
		run := ev.Header.Run
		last[run] = ts
		if running[run] == nil {
			running[run] = map[uint32]live{}
		}
		p := ev.Proc
		if p == nil {
			continue
		}
		switch p.Event {
		case record.ProcPresent, record.ProcStart:
			a := get(p.Exe)
			if p.Path != "" {
				a.paths[p.Path] = true
			}
			if a.first.IsZero() || ts.Before(a.first) {
				a.first = ts
			}
			if p.Event == record.ProcStart {
				a.p.Starts++
				a.started = true
			}
			running[run][p.PID] = live{exe: p.Exe, start: ts}
		case record.ProcExit:
			if l, ok := running[run][p.PID]; ok {
				closeOne(l, ts)
				delete(running[run], p.PID)
			}
		}
	}
	for run, procs := range running {
		for _, l := range procs {
			closeOne(l, last[run])
		}
	}

	var out []Process
	for key, a := range accs {
		fg := inFront[key]
		if !a.started && !fg {
			continue
		}
		pr := a.p
		pr.EverForeground = fg
		pr.Paths = sortedKeys(a.paths)
		pr.FirstSeen = a.first.Format("15:04:05")
		pr.Match = describe(cfg.MatchProcess(pr.Exe))
		out = append(out, pr)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RunningSeconds != out[j].RunningSeconds {
			return out[i].RunningSeconds > out[j].RunningSeconds
		}
		return out[i].Exe < out[j].Exe
	})
	return out
}

func describe(r *config.Rule) string {
	if r == nil {
		return "-"
	}
	return r.App + "/" + r.Class + "/" + r.Role
}

func sortedKeys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Text renders the report. top limits each table (0 = no limit).
func Text(r Report, top int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Inventory %s .. %s  users: %s\n", r.From, r.To, strings.Join(r.Users, ", "))
	b.WriteString("\nFOREGROUND (credited time; 'with input' = input within the idle threshold)\n")
	for i, f := range r.Foreground {
		if top > 0 && i >= top {
			fmt.Fprintf(&b, "  ... %d more (use -top 0)\n", len(r.Foreground)-top)
			break
		}
		exe := f.Exe
		if exe == "" {
			exe = "(no foreground window)"
		}
		fmt.Fprintf(&b, "  %-32s %8s  with input %8s  match %s\n", exe, dur(f.Seconds), dur(f.PresentSeconds), f.Match)
		for _, p := range f.Paths {
			fmt.Fprintf(&b, "      path   %s\n", p)
		}
		for _, t := range f.Titles {
			fmt.Fprintf(&b, "      title  %-60q %s\n", clip(t.Title, 60), dur(t.Seconds))
		}
	}
	b.WriteString("\nPROCESSES started while collecting, or ever in front (running time summed over instances)\n")
	for i, p := range r.Processes {
		if top > 0 && i >= top {
			fmt.Fprintf(&b, "  ... %d more (use -top 0)\n", len(r.Processes)-top)
			break
		}
		fg := ""
		if p.EverForeground {
			fg = "  [foreground]"
		}
		fmt.Fprintf(&b, "  %-32s %8s  starts %-3d first %s  match %s%s\n", p.Exe, dur(p.RunningSeconds), p.Starts, p.FirstSeen, p.Match, fg)
		for _, path := range p.Paths {
			fmt.Fprintf(&b, "      path   %s\n", path)
		}
	}
	return b.String()
}

func dur(s float64) string {
	d := time.Duration(s * float64(time.Second)).Round(time.Second)
	h, m, sec := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm%02ds", m, sec)
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
