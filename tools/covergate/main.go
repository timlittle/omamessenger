package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run checks the profile named in args and returns the process exit code:
// 0 when every gate passes, 1 on a missed gate, 2 on a usage or read error.
func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("covergate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root containing "+MainFile)
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: covergate [-root DIR] <cover profile>")
		return 2
	}
	results, err := check(*root, flags.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, "covergate:", err)
		return 2
	}
	if !Render(stdout, results) {
		return 1
	}
	return 0
}

func check(root, profilePath string) ([]Result, error) {
	exclude, err := mainExclusion(root)
	if err != nil {
		return nil, err
	}
	profile, err := os.Open(profilePath)
	if err != nil {
		return nil, err
	}
	defer profile.Close()
	counts, err := ParseProfile(profile, exclude)
	if err != nil {
		return nil, err
	}
	return Evaluate(counts), nil
}

func mainExclusion(root string) (lineRange, error) {
	src, err := os.ReadFile(filepath.Join(root, MainFile))
	if errors.Is(err, os.ErrNotExist) {
		return lineRange{start: 1, end: 0}, nil
	}
	if err != nil {
		return lineRange{}, err
	}
	return MainRange(src)
}
