package main

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"time"
)

const helperVersion = "0.2.0"

type Config struct {
	Demo    bool
	Chatter bool
	Seed    int64
	DataDir string
	DBPath  string
	Version bool
}

func resolveConfig(args []string, env func(string) string) (Config, error) {
	if env == nil {
		return Config{}, errors.New("environment lookup is required")
	}
	fs := flag.NewFlagSet("oma-messenger-service", flag.ContinueOnError)
	fs.SetOutput(discardWriter{})
	var cfg Config
	var noChatter bool
	fs.BoolVar(&cfg.Demo, "demo", false, "run the seeded local demo connector")
	fs.BoolVar(&noChatter, "no-chatter", false, "disable scripted demo messages")
	fs.Int64Var(&cfg.Seed, "seed", time.Now().UnixNano(), "random seed for demo behavior")
	fs.StringVar(&cfg.DataDir, "data-dir", "", "application data directory")
	fs.StringVar(&cfg.DBPath, "db", "", "database file (overrides data directory default)")
	fs.BoolVar(&cfg.Version, "version", false, "print helper version and exit")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	cfg.Chatter = !noChatter
	if cfg.DataDir == "" {
		cfg.DataDir = env("XDG_DATA_HOME")
		if cfg.DataDir == "" {
			home := env("HOME")
			if home == "" {
				return Config{}, errors.New("HOME or XDG_DATA_HOME must be set")
			}
			cfg.DataDir = filepath.Join(home, ".local", "share")
		}
		cfg.DataDir = filepath.Join(cfg.DataDir, "omamessenger")
	}
	if cfg.DBPath == "" {
		name := "messages.db"
		if cfg.Demo {
			name = "demo.db"
		}
		cfg.DBPath = filepath.Join(cfg.DataDir, name)
	}
	return cfg, nil
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
