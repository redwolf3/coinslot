package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/redwolf3/coinslot/core/activity"
	"github.com/redwolf3/coinslot/core/config"
	"github.com/redwolf3/coinslot/core/summary"
)

// summaryCmd prints the daily summary for the given log files or directories
// (default: today's log in logDir). It needs no Win32 access, so it also runs on
// copied logs on any OS.
func summaryCmd(args []string, cfgPath, logDir string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("summary", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "print JSON instead of text")
	date := fs.String("date", "", "only this local date (YYYY-MM-DD)")
	idle := fs.Float64("idle", 0, "override idle threshold in seconds (default: config)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *date != "" {
		if _, err := time.Parse("2006-01-02", *date); err != nil {
			return fmt.Errorf("-date must be YYYY-MM-DD: %q", *date)
		}
	}
	cfg, _, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	all, err := loadLogs(fs.Args(), logDir, *date, stderr)
	if err != nil {
		return err
	}

	days := summary.Summarize(activity.Replay(all, cfg, activity.Options{IdleThresholdS: *idle}))
	if *date != "" {
		var keep []summary.Day
		for _, d := range days {
			if d.Date == *date {
				keep = append(keep, d)
			}
		}
		days = keep
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if days == nil {
			days = []summary.Day{}
		}
		return enc.Encode(days)
	}
	if len(days) == 0 {
		fmt.Fprintln(stdout, "no samples found")
		return nil
	}
	_, err = io.WriteString(stdout, summary.Text(days))
	return err
}

// loadLogs parses the given log files/directories, in order, reporting malformed
// lines to stderr. With no paths it reads one day's log in logDir: date, or today.
// With date set, directories contribute only that day's file: each daily file is
// self-contained (a new run starts at local midnight), so no other day is needed.
// Files named explicitly are always read.
func loadLogs(paths []string, logDir, date string, stderr io.Writer) ([]activity.Event, error) {
	if len(paths) == 0 {
		day := date
		if day == "" {
			day = time.Now().Format("2006-01-02")
		}
		paths = []string{filepath.Join(logDir, day+".jsonl")}
	}
	files, err := expand(paths, date)
	if err != nil {
		return nil, err
	}
	var all []activity.Event
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		evs, st, err := activity.Parse(bytes.NewReader(b))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if st.Malformed > 0 {
			fmt.Fprintf(stderr, "%s: %d malformed line(s) skipped\n", f, st.Malformed)
		}
		all = append(all, evs...)
	}
	return all, nil
}

// expand turns directories into their *.jsonl files, sorted so each day's
// records stay in order. A non-empty date limits directories to <date>.jsonl.
func expand(paths []string, date string) ([]string, error) {
	var out []string
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !fi.IsDir() {
			out = append(out, p)
			continue
		}
		pattern := "*.jsonl"
		if date != "" {
			pattern = date + ".jsonl"
		}
		m, err := filepath.Glob(filepath.Join(p, pattern))
		if err != nil {
			return nil, err
		}
		sort.Strings(m)
		out = append(out, m...)
	}
	return out, nil
}
