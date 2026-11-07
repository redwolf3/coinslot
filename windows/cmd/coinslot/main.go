// Command coinslot is the Windows collector.
//
//	coinslot [run]   sample every N seconds and append JSONL to the log dir (default)
//	coinslot once    print one sample record to stdout
//	coinslot procs   list processes in this session with their config classification
//	coinslot summary [-json] [-date YYYY-MM-DD] [-idle S] [log files or dirs...]
//	                 daily summary per child (default: today's log for this user)
//	coinslot timeline [-json] [-date YYYY-MM-DD] [-idle S] [log files or dirs...]
//	                 per-child segments of identical state, to compare with notes
//	coinslot inventory [-json] [-top N] [log files or dirs...]
//	                 foreground programs and started processes, to find real exe names
//	coinslot version
//
// Build twice: coinslot.exe (console, for diagnostics) and coinslotw.exe
// (-ldflags=-H=windowsgui, no console window, for the scheduled task).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"sort"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/redwolf3/coinslot/core/config"
	"github.com/redwolf3/coinslot/core/record"
	"github.com/redwolf3/coinslot/windows/collector"
)

var version = "dev"

func main() {
	fs := flag.NewFlagSet("coinslot", flag.ExitOnError)
	cfgPath := fs.String("config", defaultConfigPath(), "path to config.json")
	logDir := fs.String("logdir", defaultLogDir(), "directory for daily JSONL logs")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: coinslot [flags] [run|once|procs|summary|timeline|inventory|version]")
		fs.PrintDefaults()
	}
	_ = fs.Parse(os.Args[1:])
	cmd := "run"
	if fs.NArg() > 0 {
		cmd = fs.Arg(0)
	}

	var err error
	switch cmd {
	case "run":
		err = run(*cfgPath, *logDir)
	case "once":
		err = once(*cfgPath)
	case "procs":
		err = procs(*cfgPath)
	case "inventory":
		err = inventoryCmd(fs.Args()[1:], *cfgPath, *logDir, os.Stdout, os.Stderr)
	case "summary":
		err = summaryCmd(fs.Args()[1:], *cfgPath, *logDir, os.Stdout, os.Stderr)
	case "timeline":
		err = timelineCmd(fs.Args()[1:], *cfgPath, *logDir, os.Stdout, os.Stderr)
	case "version":
		fmt.Println(version)
	default:
		fs.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "coinslot:", err)
		os.Exit(1)
	}
}

func defaultConfigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(filepath.Dir(exe), "config.json")
}

func defaultLogDir() string {
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		return filepath.Join(d, "Coinslot", "logs")
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "coinslot", "logs")
	}
	return "logs"
}

func newCollector(cfgPath string, sink collector.Sink) (*collector.Collector, error) {
	cfg, sum, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}
	probe, sessionID, err := newPlatform()
	if err != nil {
		return nil, err
	}
	id := collector.Identity{SessionID: sessionID, PID: os.Getpid(), Version: version}
	if u, err := user.Current(); err == nil {
		id.User, id.SID = u.Username, u.Uid // on Windows, Uid is the SID string
	}
	id.Host, _ = os.Hostname()
	return &collector.Collector{Cfg: cfg, ConfigPath: cfgPath, ConfigSHA: sum, Probe: probe, Sink: sink, ID: id}, nil
}

func run(cfgPath, logDir string) error {
	sink := collector.FileSink{Dir: logDir}
	ok, err := singleInstance("CoinslotCollector")
	if err != nil {
		return err
	}
	if !ok {
		return nil // another collector already runs in this session
	}
	c, err := newCollector(cfgPath, sink)
	if err != nil {
		// Leave a trace where the parent will look; there is no console under coinslotw.exe.
		now := time.Now()
		_ = sink.Write(record.Error{Header: record.Header{V: record.SchemaVersion, Type: record.TypeError,
			TS: now.Format(record.TimeLayout)}, Op: "startup", Msg: err.Error()}, now)
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	c.Run(ctx)
	return nil
}

// stdoutSamples prints only sample records.
type stdoutSamples struct{}

func (stdoutSamples) Write(rec any, _ time.Time) error {
	if _, ok := rec.(record.Sample); !ok {
		return nil
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Println(string(b))
	return err
}

func once(cfgPath string) error {
	c, err := newCollector(cfgPath, stdoutSamples{})
	if err != nil {
		return err
	}
	c.Start()
	c.Tick()
	return nil
}

func procs(cfgPath string) error {
	cfg, _, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	probe, sessionID, err := newPlatform()
	if err != nil {
		return err
	}
	ps, err := probe.SessionProcesses()
	if err != nil {
		return err
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].Exe < ps[j].Exe })
	fmt.Printf("session %d, %d processes\n", sessionID, len(ps))
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PID\tEXE\tMATCH\tPATH")
	for _, p := range ps {
		m := "-"
		if r := cfg.MatchProcess(p.Exe); r != nil {
			m = r.App + "/" + r.Class + "/" + r.Role
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", p.PID, p.Exe, m, probe.ProcessPath(p.PID))
	}
	return w.Flush()
}
