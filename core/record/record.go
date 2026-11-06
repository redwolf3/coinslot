// Package record defines the JSONL log record schema written by collectors and
// read by the core. It is pure and platform-independent.
package record

// SchemaVersion is written as "v" on every line. Bump only on breaking changes.
const SchemaVersion = 1

// TimeLayout is the "ts" format: local wall time with offset, millisecond precision.
const TimeLayout = "2006-01-02T15:04:05.000Z07:00"

const (
	TypeStart  = "start"
	TypeSample = "sample"
	TypeProc   = "proc"
	TypeError  = "error"
	TypeStop   = "stop"
)

// Foreground classes in Sample.FgClass (in addition to config.ClassMetered/ClassAllowed).
const (
	FgClassUnknown = "unknown" // a foreground process exists but matches no rule
	FgClassNone    = "none"    // no foreground window (lock screen, desktop transitions)
)

const (
	ProcPresent = "present" // already running when the collector started
	ProcStart   = "start"
	ProcExit    = "exit"
)

// Header is common to every record.
type Header struct {
	V    int    `json:"v"`
	Type string `json:"type"`
	TS   string `json:"ts"`
	Run  string `json:"run"`
	Seq  uint64 `json:"seq"`
	User string `json:"user"`
	SID  string `json:"sid"`
}

// Start.Reason values.
const (
	StartStartup       = "startup"        // collector process started
	StartNewDay        = "new_day"        // local date changed; each daily file is self-contained
	StartWriteRecovery = "write_recovery" // a previous write failed; process state re-snapshotted
)

type Start struct {
	Header
	Reason         string `json:"reason,omitempty"`
	Host           string `json:"host"`
	SessionID      uint32 `json:"session_id"`
	PID            int    `json:"pid"`
	Version        string `json:"version"`
	ConfigPath     string `json:"config_path"`
	ConfigSHA256   string `json:"config_sha256"`
	IntervalS      int    `json:"interval_s"`
	IdleThresholdS int    `json:"idle_threshold_s"`
}

type Sample struct {
	Header
	DTS            float64  `json:"dt_s"`
	ConsoleActive  bool     `json:"console_active"`
	FgPID          uint32   `json:"fg_pid"`
	FgExe          string   `json:"fg_exe"`
	FgPath         string   `json:"fg_path,omitempty"`
	FgTitle        string   `json:"fg_title"`
	FgHostExe      string   `json:"fg_host_exe,omitempty"`
	FgApp          string   `json:"fg_app,omitempty"`
	FgClass        string   `json:"fg_class"`
	FgRole         string   `json:"fg_role,omitempty"`
	IsMetered      bool     `json:"is_metered"`
	PlayerRunning  bool     `json:"player_running"`
	PlayersRunning []string `json:"players_running,omitempty"`
	// IdleS is seconds since the last keyboard/mouse input; nil (omitted) when the
	// read failed, so a failure is never mistaken for recent input.
	IdleS *float64 `json:"idle_s,omitempty"`
}

type Proc struct {
	Header
	Event string `json:"event"`
	PID   uint32 `json:"pid"`
	Exe   string `json:"exe"`
	Path  string `json:"path,omitempty"`
	// Titles, on "present" records of a run that follows another run of the same
	// collector, are the distinct foreground window titles seen for this process
	// (oldest first, at most collector.MaxTitles). Raw evidence that lets replay
	// re-identify title-qualified apps (e.g. Minecraft Java) across run boundaries,
	// even if the latest title no longer matches (e.g. back in a menu).
	Titles []string `json:"titles,omitempty"`
}

type Error struct {
	Header
	Op  string `json:"op"`
	Msg string `json:"msg"`
}

type Stop struct {
	Header
	Reason string `json:"reason"`
}
