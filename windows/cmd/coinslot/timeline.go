package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/redwolf3/coinslot/core/activity"
	"github.com/redwolf3/coinslot/core/config"
	"github.com/redwolf3/coinslot/core/timeline"
)

// timelineCmd prints, per child and local date, the stretches of identical state
// between samples, to compare against hand-written notes. It takes the same
// arguments as summary.
func timelineCmd(args []string, cfgPath, logDir string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("timeline", flag.ContinueOnError)
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

	days := timeline.Build(activity.Replay(all, cfg, activity.Options{IdleThresholdS: *idle}))
	if *date != "" {
		var keep []timeline.Day
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
			days = []timeline.Day{}
		}
		return enc.Encode(days)
	}
	if len(days) == 0 {
		fmt.Fprintln(stdout, "no samples found")
		return nil
	}
	_, err = io.WriteString(stdout, timeline.Text(days))
	return err
}
