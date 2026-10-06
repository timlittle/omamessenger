package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"
)

// helperVersion is this helper's release. It must match the helper-version
// file, which the installer uses to download the matching release.
const helperVersion = "0.3.0"

// config is the helper's command-line configuration.
type config struct {
	demo    bool
	chatter bool
	seed    uint64
	dataDir string
	dbPath  string
	version bool
}

// resolveConfig parses the command line, filling in the data directory from
// the XDG base directory rules when it is not given.
func resolveConfig(args []string, env func(string) string) (config, error) {
	var cfg config

	fs := flag.NewFlagSet("oma-messenger-service", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&cfg.demo, "demo", false, "run the seeded demo accounts")
	fs.BoolVar(&cfg.chatter, "chatter", false, "let the demo accounts send scripted messages in the background")
	fs.Uint64Var(&cfg.seed, "seed", uint64(time.Now().UnixNano()), "random seed for demo chatter")
	fs.StringVar(&cfg.dataDir, "data-dir", "", "data directory")
	fs.StringVar(&cfg.dbPath, "db", "", "database file, overriding the data directory")
	fs.BoolVar(&cfg.version, "version", false, "print the helper version and exit")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	return withPaths(cfg, env)
}

// withPaths fills in the data directory and database file. The demo uses
// its own database so it never mixes with real messages.
func withPaths(cfg config, env func(string) string) (config, error) {
	if cfg.dataDir == "" {
		base := env("XDG_DATA_HOME")
		if base == "" && env("HOME") == "" {
			return config{}, errors.New("HOME or XDG_DATA_HOME must be set")
		}

		if base == "" {
			base = filepath.Join(env("HOME"), ".local", "share")
		}

		cfg.dataDir = filepath.Join(base, "omamessenger")
	}

	if cfg.dbPath == "" {
		name := "messages.db"
		if cfg.demo {
			name = "demo.db"
		}

		cfg.dbPath = filepath.Join(cfg.dataDir, name)
	}

	return cfg, nil
}
