// Package collector is the platform-neutral sampling loop. All OS access goes
// through Probe so the loop can be tested on any OS with a fake.
package collector

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sort"
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

func (c *Collector) emit(rec any, ts time.Time) {
	// A failed write cannot be logged to the same sink; drop it and keep sampling.
	_ = c.Sink.Write(rec, ts)
}

// Start writes the start record. Call once before Tick.
func (c *Collector) Start() {
	if c.run == "" {
		c.run = newRunID()
	}
	c.lastErr = map[string]string{}
	ts := c.now()
	c.emit(record.Start{
		Header: c.header(record.TypeStart, ts),
		Host:   c.ID.Host, SessionID: c.ID.SessionID, PID: c.ID.PID, Version: c.ID.Version,
		ConfigPath: c.ConfigPath, ConfigSHA256: c.ConfigSHA,
		IntervalS: c.Cfg.SampleIntervalSeconds, IdleThresholdS: c.Cfg.IdleThresholdSeconds,
	}, ts)
}

// Tick takes one sample: process diff events first, then the sample record.
func (c *Collector) Tick() {
	ts := c.now()
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
		c.emit(record.Proc{Header: c.header(record.TypeProc, ts), Event: record.ProcExit, PID: k.pid, Exe: k.exe}, ts)
	}
	ev := record.ProcStart
	if first {
		ev = record.ProcPresent
	}
	for _, k := range started {
		c.emit(record.Proc{Header: c.header(record.TypeProc, ts), Event: ev, PID: k.pid, Exe: k.exe, Path: c.Probe.ProcessPath(k.pid)}, ts)
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
