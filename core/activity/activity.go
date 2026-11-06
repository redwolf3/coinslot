package activity

import (
	"sort"
	"time"

	"github.com/redwolf3/coinslot/core/config"
	"github.com/redwolf3/coinslot/core/record"
)

// Facts are the raw observations from one sample plus the process names
// running in the session at that moment.
type Facts struct {
	ConsoleActive bool
	FgPID         uint32
	FgExe         string
	FgTitle       string
	IdleS         float64
	ProcExes      []string
}

// State is the classification of one moment. The fields are not mutually
// exclusive: Roblox can be open in the background while Duolingo is in use.
type State struct {
	Present bool     // console session active and input within the idle threshold
	Active  string   // metered app being actively played (empty if none)
	Allowed string   // allowed app in active use (empty if none)
	Open    []string // metered apps open (player or launcher) but not actively played; sorted
}

// Classify applies the active-play rule. It uses only raw facts and the given
// config, never the collector's derived fields, so logs can be re-classified
// after the config is corrected.
//
// Active play for app A requires all of:
//  1. A's player process is running — implied by 2, since the foreground process is it;
//  2. the foreground window belongs to a process matching A's metered player rule;
//  3. input within idleThresholdS, in the active console session.
func Classify(cfg *config.Config, idleThresholdS float64, f Facts) State {
	var st State
	st.Present = f.ConsoleActive && f.IdleS <= idleThresholdS

	open := map[string]bool{}
	for _, exe := range f.ProcExes {
		if r := cfg.MatchProcess(exe); r != nil && r.Class == config.ClassMetered {
			open[r.App] = true
		}
	}
	if f.FgPID != 0 {
		if r := cfg.Match(f.FgExe, f.FgTitle); r != nil {
			switch {
			case r.IsMeteredPlayer() && st.Present:
				st.Active = r.App
			case r.Class == config.ClassMetered:
				open[r.App] = true
			case r.Class == config.ClassAllowed && st.Present:
				st.Allowed = r.App
			}
		}
	}
	delete(open, st.Active)
	for app := range open {
		st.Open = append(st.Open, app)
	}
	sort.Strings(st.Open)
	return st
}

// Interval is credited time ending at a sample, classified by that sample's state.
type Interval struct {
	User    string
	SID     string
	Run     string
	End     time.Time // sample wall time, with the local offset it was logged in
	Day     string    // local date of End, YYYY-MM-DD
	Seconds float64
	State
	// Raw foreground facts from the sample, for reports that need more than the State
	// (inventory, and app usage later).
	FgExe   string
	FgTitle string
	FgPath  string
}

// Options control replay. Zero values fall back to the config / defaults.
type Options struct {
	// IdleThresholdS overrides cfg.IdleThresholdSeconds when > 0.
	IdleThresholdS float64
	// MaxCreditFactor caps one sample's credit at factor × the run's sample interval,
	// so sleep/hibernate or a stalled collector never credits a long gap. Default 2.
	MaxCreditFactor float64
}

type runState struct {
	intervalS float64
	procs     map[uint32]string
}

// Replay reconstructs per-run process state from proc events and turns each
// sample into an Interval. Events must be in log order per run (runs may interleave).
//
// Credit model: a sample credits min(dt_s, cap) seconds to the state it observed,
// covering the interval since the previous sample of the same run. The first sample
// of a run has dt_s=0 and credits nothing.
func Replay(evs []Event, cfg *config.Config, opt Options) []Interval {
	threshold := opt.IdleThresholdS
	if threshold <= 0 {
		threshold = float64(cfg.IdleThresholdSeconds)
	}
	factor := opt.MaxCreditFactor
	if factor <= 0 {
		factor = 2
	}
	runs := map[string]*runState{}
	get := func(run string) *runState {
		rs := runs[run]
		if rs == nil {
			rs = &runState{intervalS: float64(cfg.SampleIntervalSeconds), procs: map[uint32]string{}}
			runs[run] = rs
		}
		return rs
	}

	var out []Interval
	for _, ev := range evs {
		rs := get(ev.Header.Run)
		switch {
		case ev.Start != nil:
			if ev.Start.IntervalS > 0 {
				rs.intervalS = float64(ev.Start.IntervalS)
			}
		case ev.Proc != nil:
			switch ev.Proc.Event {
			case record.ProcPresent, record.ProcStart:
				rs.procs[ev.Proc.PID] = ev.Proc.Exe
			case record.ProcExit:
				delete(rs.procs, ev.Proc.PID)
			}
		case ev.Sample != nil:
			s := ev.Sample
			end, err := time.Parse(record.TimeLayout, s.TS)
			if err != nil {
				continue
			}
			credit := s.DTS
			if limit := factor * rs.intervalS; credit > limit {
				credit = limit
			}
			if credit < 0 {
				credit = 0
			}
			exes := make([]string, 0, len(rs.procs))
			for _, exe := range rs.procs {
				exes = append(exes, exe)
			}
			st := Classify(cfg, threshold, Facts{
				ConsoleActive: s.ConsoleActive, FgPID: s.FgPID, FgExe: s.FgExe,
				FgTitle: s.FgTitle, IdleS: s.IdleS, ProcExes: exes,
			})
			iv := Interval{User: s.User, SID: s.SID, Run: s.Run, End: end, Seconds: credit, State: st,
				FgExe: s.FgExe, FgTitle: s.FgTitle, FgPath: s.FgPath}
			if s.FgPID == 0 {
				iv.FgExe, iv.FgTitle, iv.FgPath = "", "", ""
			}
			out = append(out, splitAtMidnight(iv)...)
		}
	}
	return out
}

// splitAtMidnight sets Day and splits an interval that starts on the previous
// local day, so each day gets exactly its own seconds.
func splitAtMidnight(iv Interval) []Interval {
	iv.Day = iv.End.Format("2006-01-02")
	start := iv.End.Add(-time.Duration(iv.Seconds * float64(time.Second)))
	midnight := time.Date(iv.End.Year(), iv.End.Month(), iv.End.Day(), 0, 0, 0, 0, iv.End.Location())
	if !start.Before(midnight) {
		return []Interval{iv}
	}
	before := iv
	before.End = midnight
	before.Day = start.Format("2006-01-02")
	before.Seconds = midnight.Sub(start).Seconds()
	iv.Seconds -= before.Seconds
	return []Interval{before, iv}
}
