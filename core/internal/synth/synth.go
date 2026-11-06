// Package synth builds synthetic collector logs for tests.
package synth

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/redwolf3/coinslot/core/record"
)

// Log writes records the way the collector does: one run, monotonic seq,
// samples every Interval with dt_s measured from the previous sample.
type Log struct {
	User, SID, Run string
	Interval       time.Duration
	Now            time.Time

	buf      bytes.Buffer
	seq      uint64
	prev     time.Time
	nextPID  uint32
	pidByExe map[string]uint32
	started  bool
}

func New(user, sid, run string, start time.Time) *Log {
	return &Log{User: user, SID: sid, Run: run, Interval: 5 * time.Second, Now: start,
		nextPID: 100, pidByExe: map[string]uint32{}}
}

func (l *Log) hdr(typ string) record.Header {
	l.seq++
	return record.Header{V: record.SchemaVersion, Type: typ, TS: l.Now.Format(record.TimeLayout),
		Run: l.Run, Seq: l.seq, User: l.User, SID: l.SID}
}

func (l *Log) write(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	l.buf.Write(b)
	l.buf.WriteByte('\n')
}

// Start writes the start record.
func (l *Log) Start() *Log {
	l.started = true
	l.write(record.Start{Header: l.hdr(record.TypeStart), IntervalS: int(l.Interval / time.Second), IdleThresholdS: 30})
	return l
}

// FakePath is the image path synth logs use for exe.
func FakePath(exe string) string { return `C:\Apps\` + exe }

// Launch writes a proc start event and returns the pid.
func (l *Log) Launch(exe string) uint32 { return l.proc(record.ProcStart, exe) }

// Present writes a proc "present" event (already running at collector start).
func (l *Log) Present(exe string) uint32 { return l.proc(record.ProcPresent, exe) }

func (l *Log) proc(event, exe string) uint32 {
	l.nextPID++
	l.pidByExe[exe] = l.nextPID
	l.write(record.Proc{Header: l.hdr(record.TypeProc), Event: event, PID: l.nextPID, Exe: exe, Path: FakePath(exe)})
	return l.nextPID
}

// Exit writes a proc exit event for the last pid launched with exe.
func (l *Log) Exit(exe string) {
	pid := l.pidByExe[exe]
	delete(l.pidByExe, exe)
	l.write(record.Proc{Header: l.hdr(record.TypeProc), Event: record.ProcExit, PID: pid, Exe: exe})
}

// Fg describes the foreground for a stretch of samples. Exe "" means no foreground window.
type Fg struct {
	Exe, Title string
	IdleS      float64
	Away       bool // session not the active console (user switched away)
	// IdleUnknown omits idle_s, as the collector does when the idle read fails.
	IdleUnknown bool
}

// Samples writes n samples, advancing time by Interval before each one except the
// very first sample of the run.
func (l *Log) Samples(n int, fg Fg) *Log {
	for i := 0; i < n; i++ {
		var dt float64
		if !l.prev.IsZero() {
			l.Now = l.Now.Add(l.Interval)
			dt = l.Now.Sub(l.prev).Seconds()
		}
		l.prev = l.Now
		var pid uint32
		var path string
		if fg.Exe != "" {
			path = FakePath(fg.Exe)
			pid = l.pidByExe[fg.Exe]
			if pid == 0 {
				pid = 1 // foreground process not tracked (e.g. explorer)
			}
		}
		var idle *float64
		if !fg.IdleUnknown {
			v := fg.IdleS
			idle = &v
		}
		l.write(record.Sample{Header: l.hdr(record.TypeSample), DTS: dt, ConsoleActive: !fg.Away,
			FgPID: pid, FgExe: fg.Exe, FgPath: path, FgTitle: fg.Title, FgClass: "ignored-by-core", IdleS: idle})
	}
	return l
}

// Gap advances wall time without samples (sleep, logoff). The next sample's dt_s
// reflects the whole gap, as a monotonic clock that counts suspend would.
func (l *Log) Gap(d time.Duration) *Log {
	l.Now = l.Now.Add(d)
	return l
}

// Raw appends an arbitrary line (for malformed-input tests).
func (l *Log) Raw(s string) *Log {
	l.buf.WriteString(s)
	l.buf.WriteByte('\n')
	return l
}

func (l *Log) Bytes() []byte { return l.buf.Bytes() }
