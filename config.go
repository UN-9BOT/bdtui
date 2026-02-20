package main

import (
	"errors"
	"flag"
	"io"
)

type Config struct {
	BeadsDir string
	NoWatch  bool
}

func parseConfig(args []string) (Config, error) {
	fs := flag.NewFlagSet("bdtui", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var cfg Config
	fs.StringVar(&cfg.BeadsDir, "beads-dir", "", "Path to .beads directory")
	fs.BoolVar(&cfg.NoWatch, "no-watch", false, "Disable periodic auto-reload")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() > 0 {
		return Config{}, errors.New("unexpected positional arguments")
	}

	return cfg, nil
}
