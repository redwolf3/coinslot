package collector

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/redwolf3/coinslot/core/config"
	"github.com/redwolf3/coinslot/core/record"
)

type fakeProbe struct {
	fg       Foreground
	fgErr    error
	idle     float64
	procs    []Proc
	procsErr error
	console  bool
}

func (f *fakeProbe) Foreground() (Foreground, error)   { return f.fg, f.fgErr }
func (f *fakeProbe) IdleSeconds() (float64, error)     { return f.idle, nil }
func (f *fakeProbe) SessionProcesses() ([]Proc, error) { return f.procs, f.procsErr }
func (f *fakeProbe) ProcessPath(pid uint32) string     { return `C:\fake\` + string(rune('a'+pid%26)) }
func (f *fakeProbe) ConsoleActive() bool               { return f.console }

type memSink struct {
	recs []map[string]any
	fail bool // reject writes, as a full disk or locked file would
}

func (m *memSink) Write(rec any, _ time.Time) error {
	if m.fail {
		return errors.New("disk full")
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	m.recs = append(m.recs, v)
	return nil
}

func (m *memSink) ofType(t string) []map[string]any {
	var out []map[string]any
	for _, r := range m.recs {
		if r["type"] == t {
			out = append(out, r)
		}
	}
	return out
}

const cfgJSON = `{"schema":1,"apps":[
 {"app":"roblox","class":"metered","role":"player","exe":["RobloxPlayerBeta.exe"]},
 {"app":"roblox","class":"metered","role":"launcher","exe":["RobloxPlayerInstaller.exe"]},
 {"app":"minecraft","class":"metered","role":"player","exe":["javaw.exe"],"title_contains":["Minecraft"]},
 {"app":"duolingo","class":"allowed","exe":["msedge.exe"],"title_contains":["Duolingo"]}]}`

func newTest(t *testing.T) (*Collector, *fakeProbe, *memSink, *time.Time) {
	t.Helper()
	cfg, err := config.Parse([]byte(cfgJSON))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 6, 19, 0, 0, 0, time.FixedZone("EST", -5*3600))
	p := &fakeProbe{console: true}
	s := &memSink{}
	c := &Collector{Cfg: cfg, Probe: p, Sink: s, ID: Identity{User: `PC\kid1`, SID: "S-1-5-21-1"},
		Now: func() time.Time { return now }}
	c.Start()
	return c, p, s, &now
}

func TestSampleClassification(t *testing.T) {
	c, p, s, now := newTest(t)
	cases := []struct {
		name          string
		fg            Foreground
		procs         []Proc
		wantClass     string
		wantRole      string
		wantMetered   bool
		wantPlayerRun bool
		wantPlayers   []any
	}{
		{"none", Foreground{}, nil, "none", "", false, false, nil},
		{"unknown", Foreground{PID: 1, Exe: "explorer.exe"}, nil, "unknown", "", false, false, nil},
		{"launcher fg, no player", Foreground{PID: 2, Exe: "RobloxPlayerInstaller.exe"},
			[]Proc{{2, "RobloxPlayerInstaller.exe"}}, "metered", "launcher", true, false, nil},
		{"player fg", Foreground{PID: 3, Exe: "RobloxPlayerBeta.exe", Title: "Roblox"},
			[]Proc{{3, "RobloxPlayerBeta.exe"}}, "metered", "player", true, true, []any{"roblox"}},
		{"allowed fg, player in background", Foreground{PID: 4, Exe: "msedge.exe", Title: "Duolingo"},
			[]Proc{{3, "RobloxPlayerBeta.exe"}, {4, "msedge.exe"}}, "allowed", "player", false, true, []any{"roblox"}},
		{"title-qualified player only via foreground", Foreground{PID: 5, Exe: "javaw.exe", Title: "Minecraft 1.21"},
			[]Proc{{5, "javaw.exe"}}, "metered", "player", true, true, []any{"minecraft"}},
		{"javaw in background is not a player", Foreground{PID: 1, Exe: "explorer.exe"},
			[]Proc{{5, "javaw.exe"}}, "unknown", "", false, false, nil},
	}
	for _, tc := range cases {
		p.fg, p.procs, p.idle = tc.fg, tc.procs, 2.5
		*now = now.Add(5 * time.Second)
		c.Tick()
		got := s.ofType("sample")[len(s.ofType("sample"))-1]
		if got["fg_class"] != tc.wantClass || (got["fg_role"] != nil && got["fg_role"] != tc.wantRole) ||
			got["is_metered"] != tc.wantMetered || got["player_running"] != tc.wantPlayerRun ||
			!reflect.DeepEqual(got["players_running"], func() any {
				if tc.wantPlayers == nil {
					return nil
				}
				return tc.wantPlayers
			}()) {
			t.Errorf("%s: got %v", tc.name, got)
		}
		if tc.wantRole == "" && got["fg_role"] != nil {
			t.Errorf("%s: unexpected role %v", tc.name, got["fg_role"])
		}
		if got["idle_s"] != 2.5 || got["user"] != `PC\kid1` || got["sid"] != "S-1-5-21-1" || got["v"] != float64(1) {
			t.Errorf("%s: header/idle wrong: %v", tc.name, got)
		}
	}
}

func TestDTAndSeq(t *testing.T) {
	c, _, s, now := newTest(t)
	c.Tick()
	*now = now.Add(5200 * time.Millisecond)
	c.Tick()
	c.Stop("test")
	samples := s.ofType("sample")
	if samples[0]["dt_s"] != float64(0) || samples[1]["dt_s"] != 5.2 {
		t.Fatalf("dt: %v %v", samples[0]["dt_s"], samples[1]["dt_s"])
	}
	if samples[1]["ts"] != "2026-11-06T19:00:05.200-05:00" {
		t.Fatalf("ts: %v", samples[1]["ts"])
	}
	run := s.recs[0]["run"]
	for i, r := range s.recs {
		if r["seq"] != float64(i+1) || r["run"] != run {
			t.Fatalf("record %d: seq/run %v/%v", i, r["seq"], r["run"])
		}
	}
	if s.recs[0]["type"] != "start" || s.recs[len(s.recs)-1]["type"] != "stop" {
		t.Fatal("start/stop framing")
	}
}

func TestProcEvents(t *testing.T) {
	c, p, s, _ := newTest(t)
	p.procs = []Proc{{1, "explorer.exe"}, {2, "msedge.exe"}}
	c.Tick()
	p.procs = []Proc{{1, "explorer.exe"}, {3, "RobloxPlayerBeta.exe"}}
	c.Tick()
	c.Tick() // no change
	type ev struct {
		Event string
		PID   float64
		Exe   string
	}
	var got []ev
	for _, r := range s.ofType("proc") {
		got = append(got, ev{r["event"].(string), r["pid"].(float64), r["exe"].(string)})
	}
	want := []ev{
		{"present", 1, "explorer.exe"}, {"present", 2, "msedge.exe"},
		{"exit", 2, "msedge.exe"}, {"start", 3, "RobloxPlayerBeta.exe"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
	if s.ofType("proc")[3]["path"] == "" {
		t.Fatal("start event should carry path")
	}
}

func TestErrorsDeduplicated(t *testing.T) {
	c, p, s, _ := newTest(t)
	p.fgErr = errors.New("access denied")
	c.Tick()
	c.Tick()
	p.fgErr = nil
	c.Tick()
	p.fgErr = errors.New("access denied")
	c.Tick()
	if n := len(s.ofType("error")); n != 2 {
		t.Fatalf("want 2 error records, got %d", n)
	}
	if n := len(s.ofType("sample")); n != 4 {
		t.Fatalf("samples must continue on error, got %d", n)
	}
}

func TestProcessErrorKeepsPreviousSet(t *testing.T) {
	c, p, s, _ := newTest(t)
	p.procs = []Proc{{1, "explorer.exe"}}
	c.Tick()
	p.procs, p.procsErr = nil, errors.New("snapshot failed")
	c.Tick()
	p.procs, p.procsErr = []Proc{{1, "explorer.exe"}}, nil
	c.Tick()
	for _, r := range s.ofType("proc") {
		if r["event"] == "exit" {
			t.Fatal("a failed snapshot must not produce exit events")
		}
	}
}

func TestFileSinkRotatesByLocalDate(t *testing.T) {
	dir := t.TempDir()
	sink := FileSink{Dir: dir}
	tz := time.FixedZone("EST", -5*3600)
	d1 := time.Date(2026, 11, 6, 23, 59, 58, 0, tz)
	d2 := d1.Add(4 * time.Second)
	for _, ts := range []time.Time{d1, d1, d2} {
		if err := sink.Write(map[string]string{"ts": ts.Format(record.TimeLayout)}, ts); err != nil {
			t.Fatal(err)
		}
	}
	count := func(p string) int {
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		n := 0
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			n++
		}
		return n
	}
	if n := count(sink.Path(d1)); n != 2 {
		t.Fatalf("day1 lines = %d", n)
	}
	if n := count(sink.Path(d2)); n != 1 {
		t.Fatalf("day2 lines = %d", n)
	}
}

func runsOf(s *memSink) []string {
	var runs []string
	for _, r := range s.ofType("start") {
		runs = append(runs, r["reason"].(string)+":"+r["run"].(string))
	}
	return runs
}

// Copilot review r4496048739: after a failed write the logged process state is
// unreliable, so the next tick must start a new run with a fresh snapshot.
func TestWriteFailureStartsNewRunWithSnapshot(t *testing.T) {
	c, p, s, now := newTest(t)
	p.procs = []Proc{{1, "explorer.exe"}}
	c.Tick()
	s.fail = true // the Roblox start event is lost
	p.procs = []Proc{{1, "explorer.exe"}, {3, "RobloxPlayerBeta.exe"}}
	*now = now.Add(5 * time.Second)
	c.Tick()
	s.fail = false
	*now = now.Add(5 * time.Second)
	c.Tick()

	starts := s.ofType("start")
	if len(starts) != 2 || starts[0]["reason"] != "startup" || starts[1]["reason"] != "write_recovery" ||
		starts[0]["run"] == starts[1]["run"] {
		t.Fatalf("starts: %v", runsOf(s))
	}
	newRun := starts[1]["run"]
	var present []string
	for _, r := range s.ofType("proc") {
		if r["run"] == newRun {
			if r["event"] != "present" {
				t.Fatalf("new run should begin with a snapshot, got %v", r)
			}
			present = append(present, r["exe"].(string))
		}
	}
	if !reflect.DeepEqual(present, []string{"explorer.exe", "RobloxPlayerBeta.exe"}) {
		t.Fatalf("snapshot = %v", present)
	}
	last := s.ofType("sample")[len(s.ofType("sample"))-1]
	if last["run"] != newRun || last["seq"] != float64(4) || last["dt_s"] != float64(0) {
		t.Fatalf("first sample of new run: %v", last)
	}
}

// Copilot review r4496048618: each daily log must be replayable on its own, so a
// run never spans midnight.
func TestNewRunAtLocalMidnight(t *testing.T) {
	c, p, s, now := newTest(t)
	*now = time.Date(2026, 11, 6, 23, 59, 55, 0, now.Location())
	p.procs = []Proc{{3, "RobloxPlayerBeta.exe"}}
	c.Tick()
	*now = now.Add(5 * time.Second) // 00:00:00 next day
	c.Tick()
	starts := s.ofType("start")
	if len(starts) != 2 || starts[1]["reason"] != "new_day" || starts[1]["ts"] != "2026-11-07T00:00:00.000-05:00" {
		t.Fatalf("starts: %v", starts)
	}
	n := 0
	for _, r := range s.ofType("proc") {
		if r["run"] == starts[1]["run"] && r["event"] == "present" && r["exe"] == "RobloxPlayerBeta.exe" {
			n++
		}
	}
	if n != 1 {
		t.Fatal("new day's run must re-snapshot running processes")
	}
}

// Copilot reviews r4496152987, r4496231028: a new run's snapshot carries the
// distinct foreground titles seen for each surviving process (bounded), so an
// earlier matching title survives a later non-matching one.
func TestNewRunSnapshotCarriesTitles(t *testing.T) {
	c, p, s, now := newTest(t)
	p.procs = []Proc{{1, "explorer.exe"}, {9, "javaw.exe"}}
	for _, title := range []string{"Minecraft 1.21.4 - Singleplayer", "Minecraft 1.21.4", "Minecraft 1.21.4 - Singleplayer"} {
		p.fg = Foreground{PID: 9, Exe: "javaw.exe", Title: title}
		c.Tick()
		*now = now.Add(5 * time.Second)
	}
	p.fg = Foreground{PID: 1, Exe: "explorer.exe"}
	c.Tick()
	s.fail = true
	*now = now.Add(5 * time.Second)
	c.Tick()
	s.fail = false
	*now = now.Add(5 * time.Second)
	c.Tick() // write_recovery run
	newRun := s.ofType("start")[1]["run"]
	got := map[string]any{}
	for _, r := range s.ofType("proc") {
		if r["run"] == newRun {
			got[r["exe"].(string)] = r["titles"]
		}
	}
	want := []any{"Minecraft 1.21.4 - Singleplayer", "Minecraft 1.21.4"}
	if !reflect.DeepEqual(got["javaw.exe"], want) || got["explorer.exe"] != nil {
		t.Fatalf("snapshot titles: %v", got)
	}
}

func TestRememberTitleBoundedAndResetOnPidReuse(t *testing.T) {
	c := &Collector{}
	for i := 0; i < MaxTitles+3; i++ {
		c.rememberTitle(9, "javaw.exe", string(rune('a'+i)))
	}
	if got := c.titles[9].titles; len(got) != MaxTitles || got[0] != "d" {
		t.Fatalf("bounded titles: %v", got)
	}
	c.rememberTitle(9, "notepad.exe", "x") // pid reused by another exe
	if got := c.titles[9]; got.exe != "notepad.exe" || !reflect.DeepEqual(got.titles, []string{"x"}) {
		t.Fatalf("after pid reuse: %+v", got)
	}
}

// Copilot review r4496313725: a title-identified process that exits at a run
// boundary has no exit diff; the new run's snapshot must expire its titles so an
// unrelated process reusing the pid (same exe) does not inherit them.
func TestTitlesExpireForExitAtRunBoundary(t *testing.T) {
	c, p, s, now := newTest(t)
	p.procs = []Proc{{1, "explorer.exe"}, {9, "javaw.exe"}}
	p.fg = Foreground{PID: 9, Exe: "javaw.exe", Title: "Minecraft 1.21.4"}
	c.Tick()
	s.fail = true // write failure: next tick starts a new run
	p.fg = Foreground{PID: 1, Exe: "explorer.exe"}
	*now = now.Add(5 * time.Second)
	c.Tick()
	s.fail = false
	p.procs = []Proc{{1, "explorer.exe"}} // Minecraft exited at the boundary
	*now = now.Add(5 * time.Second)
	c.Tick() // write_recovery run; snapshot confirms pid 9 is gone
	if _, ok := c.titles[9]; ok {
		t.Fatal("titles of an exited process survived the run boundary")
	}
	// An unrelated javaw.exe reuses pid 9; across the next boundary its snapshot
	// must not carry Minecraft titles.
	p.procs = []Proc{{1, "explorer.exe"}, {9, "javaw.exe"}}
	*now = now.Add(5 * time.Second)
	c.Tick()
	*now = time.Date(2026, 11, 7, 0, 0, 0, 0, now.Location()) // new_day run
	c.Tick()
	starts := s.ofType("start")
	last := starts[len(starts)-1]
	if last["reason"] != "new_day" {
		t.Fatalf("expected a new_day run, got %v", last)
	}
	for _, r := range s.ofType("proc") {
		if r["run"] == last["run"] && r["exe"] == "javaw.exe" && r["titles"] != nil {
			t.Fatalf("reused pid inherited titles: %v", r)
		}
	}
}

// The expiry must not drop evidence for a process seen in the foreground before
// any snapshot listed it (Copilot review 5805169230).
func TestTitlesKeptForProcessNotYetInBaseline(t *testing.T) {
	c, p, s, now := newTest(t)
	p.procs = []Proc{{1, "explorer.exe"}}
	c.Tick()
	s.fail = true
	p.fg = Foreground{PID: 9, Exe: "javaw.exe", Title: "Minecraft 1.21.4"} // not in any snapshot yet
	*now = now.Add(5 * time.Second)
	c.Tick()
	s.fail = false
	p.procs = []Proc{{1, "explorer.exe"}, {9, "javaw.exe"}}
	*now = now.Add(5 * time.Second)
	c.Tick()
	newRun := s.ofType("start")[1]["run"]
	for _, r := range s.ofType("proc") {
		if r["run"] == newRun && r["exe"] == "javaw.exe" {
			if !reflect.DeepEqual(r["titles"], []any{"Minecraft 1.21.4"}) {
				t.Fatalf("titles = %v", r["titles"])
			}
			return
		}
	}
	t.Fatal("javaw.exe missing from the new run's snapshot")
}
