// Package collector is the platform-neutral sampling loop. All OS access goes
// through Probe so the loop can be tested on any OS with a fake.
package collector

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"github.com/redwolf3/coinslot/core/config"
	"github.com/redwolf3/coinslot/core/record"
)

// Foreground describes the process owning the foreground window.
// PID 0 means there is no foreground window.
type Foreground struct {
	PID     uint32
	Exe     string
	Path    string
	Title   string
	HostExe string // set when resolved through a host such as ApplicationFrameHost.exe
}

type Proc struct {
	PID uint32
	Exe string
}

// Probe reads raw facts from the OS. It must not make classification decisions.
type Probe interface {
	Foreground() (Foreground, error)
	IdleSeconds() (float64, error)
	// SessionProcesses returns processes in the collector's own logon session only.
	SessionProcesses() ([]Proc, error)
	ProcessPath(pid uint32) string
	ConsoleActive() bool
}

type Identity struct {
	User      string
	SID       string
	Host      string
	SessionID uint32
	PID       int
	Version   string
}

type Sink interface {
	Write(rec any, ts time.Time) error
}

type Collector struct {
	Cfg        *config.Config
	ConfigPath string
	ConfigSHA  string
	Probe      Probe
	Sink       Sink
	ID         Identity
	Now        func() time.Time // defaults to time.Now

	run      string
	seq      uint64
	prevTick time.Time
	procs    map[procKey]struct{} // nil until first tick
	lastErr  map[string]string
	day      string // local date of the current run's start
	broken   bool   // a write failed in this run; the next tick starts a new run
	// lastTitle is the last foreground window title seen per running pid. It
	// survives run changes so a new run's "present" snapshot can carry it as raw
	// evidence (e.g. javaw.exe that showed a Minecraft title).
	lastTitle map[uint32]seenTitle
}

type seenTitle struct {
	exe, title string
}

type procKey struct {
	pid uint32
	exe string
}

func (c *Collector) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Collector) header(typ string, ts time.Time) record.Header {
	c.seq++
	return record.Header{
		V: record.SchemaVersion, Type: typ, TS: ts.Format(record.TimeLayout),
		Run: c.run, Seq: c.seq, User: c.ID.User, SID: c.ID.SID,
	}
}

// emit writes a record. A failed write cannot be logged to the same sink, and
// the process diff has already moved on, so the run's process state in the log is
// now unreliable: mark the run broken so the next tick starts a new run with a
// fresh process snapshot.
func (c *Collector) emit(rec any, ts time.Time) {
	if err := c.Sink.Write(rec, ts); err != nil {
		c.broken = true
	}
}

// Start begins the first run. Tick starts one automatically if needed.
func (c *Collector) Start() { c.begin(c.now(), record.StartStartup) }

// begin starts a new run: new run id, fresh sequence, empty process state (so
// the next diff logs a full "present" snapshot), and a start record. A run never
// spans two local dates or a failed write, so every daily log file and every run
// can be replayed on its own.
func (c *Collector) begin(ts time.Time, reason string) {
	c.run = newRunID()
	c.seq = 0
	c.prevTick = time.Time{}
	c.procs = nil
	c.lastErr = map[string]string{}
	c.day = ts.Format("2006-01-02")
	c.broken = false
	c.emit(record.Start{
		Header: c.header(record.TypeStart, ts), Reason: reason,
		Host: c.ID.Host, SessionID: c.ID.SessionID, PID: c.ID.PID, Version: c.ID.Version,
		ConfigPath: c.ConfigPath, ConfigSHA256: c.ConfigSHA,
		IntervalS: c.Cfg.SampleIntervalSeconds, IdleThresholdS: c.Cfg.IdleThresholdSeconds,
	}, ts)
}

// Tick takes one sample: process diff events first, then the sample record.
// It starts a new run first when there is none yet, the local date changed, or a
// write failed in the current run.
func (c *Collector) Tick() {
	ts := c.now()
	switch {
	case c.day == "":
		c.begin(ts, record.StartStartup)
	case c.broken:
		c.begin(ts, record.StartWriteRecovery)
	case ts.Format("2006-01-02") != c.day:
		c.begin(ts, record.StartNewDay)
	}
	var dt float64
	if !c.prevTick.IsZero() {
		dt = ts.Sub(c.prevTick).Seconds() // monotonic when Now is time.Now
	}
	c.prevTick = ts

	s := record.Sample{ConsoleActive: c.Probe.ConsoleActive(), DTS: round3(dt)}

	procs, err := c.Probe.SessionProcesses()
	if c.checkErr("processes", err, ts) {
		c.diffProcs(procs, ts)
	}
	players := map[string]struct{}{}
	for _, p := range procs {
		if r := c.Cfg.MatchProcess(p.Exe); r.IsMeteredPlayer() {
			players[r.App] = struct{}{}
		}
	}

	fg, err := c.Probe.Foreground()
	c.checkErr("foreground", err, ts)
	s.FgPID, s.FgExe, s.FgPath, s.FgTitle, s.FgHostExe = fg.PID, fg.Exe, fg.Path, fg.Title, fg.HostExe
	if fg.PID != 0 && fg.Title != "" {
		if c.lastTitle == nil {
			c.lastTitle = map[uint32]seenTitle{}
		}
		c.lastTitle[fg.PID] = seenTitle{exe: fg.Exe, title: fg.Title}
	}
	switch r := c.Cfg.Match(fg.Exe, fg.Title); {
	case fg.PID == 0:
		s.FgClass = record.FgClassNone
	case r == nil:
		s.FgClass = record.FgClassUnknown
	default:
		s.FgApp, s.FgClass, s.FgRole = r.App, r.Class, r.Role
		s.IsMetered = r.Class == config.ClassMetered
		// A title-qualified player (e.g. javaw.exe + "Minecraft") is only visible
		// via the foreground window, so count it as running here.
		if r.IsMeteredPlayer() {
			players[r.App] = struct{}{}
		}
	}

	for app := range players {
		s.PlayersRunning = append(s.PlayersRunning, app)
	}
	sort.Strings(s.PlayersRunning)
	s.PlayerRunning = len(s.PlayersRunning) > 0

	idle, err := c.Probe.IdleSeconds()
	if c.checkErr("idle", err, ts) {
		v := round3(idle)
		s.IdleS = &v
	}

	s.Header = c.header(record.TypeSample, ts)
	c.emit(s, ts)
}

// Stop writes the stop record.
func (c *Collector) Stop(reason string) {
	ts := c.now()
	c.emit(record.Stop{Header: c.header(record.TypeStop, ts), Reason: reason}, ts)
}

// Run samples every SampleIntervalSeconds until ctx is cancelled.
func (c *Collector) Run(ctx context.Context) {
	c.Start()
	c.Tick()
	t := time.NewTicker(time.Duration(c.Cfg.SampleIntervalSeconds) * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			c.Stop("shutdown")
			return
		case <-t.C:
			c.Tick()
		}
	}
}

func (c *Collector) diffProcs(procs []Proc, ts time.Time) {
	cur := make(map[procKey]struct{}, len(procs))
	for _, p := range procs {
		cur[procKey{p.PID, p.Exe}] = struct{}{}
	}
	first := c.procs == nil
	var started, exited []procKey
	for k := range cur {
		if _, ok := c.procs[k]; !ok {
			started = append(started, k)
		}
	}
	for k := range c.procs {
		if _, ok := cur[k]; !ok {
			exited = append(exited, k)
		}
	}
	sortKeys(exited)
	sortKeys(started)
	for _, k := range exited {
		delete(c.lastTitle, k.pid)
		c.emit(record.Proc{Header: c.header(record.TypeProc, ts), Event: record.ProcExit, PID: k.pid, Exe: k.exe}, ts)
	}
	ev := record.ProcStart
	if first {
		ev = record.ProcPresent
	}
	for _, k := range started {
		rec := record.Proc{Header: c.header(record.TypeProc, ts), Event: ev, PID: k.pid, Exe: k.exe, Path: c.Probe.ProcessPath(k.pid)}
		if seen, ok := c.lastTitle[k.pid]; ok {
			if first && strings.EqualFold(seen.exe, k.exe) {
				rec.LastTitle = seen.title
			} else if !first {
				delete(c.lastTitle, k.pid) // a new process reusing the pid
			}
		}
		c.emit(rec, ts)
	}
	c.procs = cur
}

// checkErr logs an error record when the error message for op changes, so a
// persistent failure is logged once. Recovery is silent. Returns true when err is nil.
func (c *Collector) checkErr(op string, err error, ts time.Time) bool {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if msg != c.lastErr[op] {
		c.lastErr[op] = msg
		if msg != "" {
			c.emit(record.Error{Header: c.header(record.TypeError, ts), Op: op, Msg: msg}, ts)
		}
	}
	return err == nil
}

func sortKeys(ks []procKey) {
	sort.Slice(ks, func(i, j int) bool {
		if ks[i].pid != ks[j].pid {
			return ks[i].pid < ks[j].pid
		}
		return ks[i].exe < ks[j].exe
	})
}

func round3(f float64) float64 { return float64(int64(f*1000+0.5)) / 1000 }

func newRunID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
