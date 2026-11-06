package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// FileSink appends one JSON object per line to Dir/YYYY-MM-DD.jsonl, using the
// record's local date. The file is opened per write so rotation needs no state
// and nothing holds a lock between samples.
type FileSink struct {
	Dir string
}

func (s FileSink) Path(ts time.Time) string {
	return filepath.Join(s.Dir, ts.Format("2006-01-02")+".jsonl")
}

func (s FileSink) Write(rec any, ts time.Time) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.Path(ts), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
