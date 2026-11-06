package main

import (
	"encoding/json"
	"flag"
	"io"

	"github.com/redwolf3/coinslot/core/config"
	"github.com/redwolf3/coinslot/core/inventory"
)

// inventoryCmd lists foreground programs (with window titles and paths) and
// processes started while collecting, from logs. Use it after an evening of
// collecting to find the real player/launcher exe names for config.json.
func inventoryCmd(args []string, cfgPath, logDir string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("inventory", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "print JSON instead of text")
	top := fs.Int("top", 40, "rows per table (0 = all)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, _, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	evs, err := loadLogs(fs.Args(), logDir, "", stderr)
	if err != nil {
		return err
	}
	rep := inventory.Build(evs, cfg)
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	_, err = io.WriteString(stdout, inventory.Text(rep, *top))
	return err
}
