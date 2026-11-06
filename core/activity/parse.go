// Package activity parses collector logs, reconstructs per-run process state, and
// classifies each sample into credited time intervals. It is pure and
// platform-independent.
package activity

import (
	"bufio"
	"encoding/json"
	"io"

	"github.com/redwolf3/coinslot/core/record"
)

// Event is one parsed log line. Exactly one of Start, Sample, Proc is set.
type Event struct {
	Header record.Header
	Start  *record.Start
	Sample *record.Sample
	Proc   *record.Proc
}

// ParseStats counts lines that were not turned into events.
type ParseStats struct {
	Lines     int
	Malformed int // not JSON, or wrong shape
	Skipped   int // unsupported schema version, or types the core ignores (error, stop)
}

// Parse reads JSONL records. Bad lines are counted, not fatal: a log truncated by a
// crash or power loss must still summarise.
func Parse(r io.Reader) ([]Event, ParseStats, error) {
	var evs []Event
	var st ParseStats
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		st.Lines++
		var h record.Header
		if err := json.Unmarshal(line, &h); err != nil {
			st.Malformed++
			continue
		}
		if h.V != record.SchemaVersion {
			st.Skipped++
			continue
		}
		ev := Event{Header: h}
		var err error
		switch h.Type {
		case record.TypeStart:
			ev.Start = &record.Start{}
			err = json.Unmarshal(line, ev.Start)
		case record.TypeSample:
			ev.Sample = &record.Sample{}
			err = json.Unmarshal(line, ev.Sample)
		case record.TypeProc:
			ev.Proc = &record.Proc{}
			err = json.Unmarshal(line, ev.Proc)
		default:
			st.Skipped++
			continue
		}
		if err != nil {
			st.Malformed++
			continue
		}
		evs = append(evs, ev)
	}
	return evs, st, sc.Err()
}
