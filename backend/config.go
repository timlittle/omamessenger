package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
)

// helperVersion is this helper's release. It must match the helper-version
// file, which the installer uses to download the matching release.
const helperVersion = "0.3.2" // x-release-please-version

// config is the helper's command-line configuration.
type config struct {
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

// withPaths fills in the data directory and database file.
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
		cfg.dbPath = filepath.Join(cfg.dataDir, "messages.db")
	}

	return cfg, nil
}
