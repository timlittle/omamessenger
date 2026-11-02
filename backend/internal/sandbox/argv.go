package sandbox

// argv.go builds the exact command line Run executes: prlimit, with
// its own resource-limit flags, running bwrap, with its own isolation
// flags, running the caller's binary. buildArgv and bwrapArgs are pure
// functions of their own arguments and the fixed constants in
// limits.go - never of anything a sender or the file being processed
// controls - so they are unit-tested exhaustively without starting
// any process at all (see argv_test.go).

import "strconv"

// seccompFD is the file descriptor bwrap reads its seccomp program
// from, once exec'd: os/exec always gives ExtraFiles its slots
// starting at 3, after the three standard streams, and Run always sets
// exactly one ExtraFiles entry (see process.go), so this is always
// correct, not just the common case.
const seccompFD = 3

// buildArgv returns the full command Run starts for spec: prlimit,
// named by its own resolved path, with every argument needed to run
// bwrap (bwrapPath) under this package's fixed resource limits and
// then, inside it, spec.Binary with spec.Args.
func buildArgv(spec Spec, bwrapPath, prlimitPath string) (name string, args []string) {
	args = []string{
		"--cpu=" + strconv.Itoa(maxCPUSeconds),
		"--as=" + strconv.Itoa(maxAddressSpaceBytes),
		"--fsize=" + strconv.Itoa(maxOutputFileBytes),
		"--nofile=" + strconv.Itoa(maxOpenFiles),
		"--",
		bwrapPath,
	}
	args = append(args, bwrapArgs(spec)...)
	args = append(args, "--", spec.Binary)
	args = append(args, spec.Args...)

	return prlimitPath, args
}

// bwrapArgs returns every bwrap flag Run passes for spec, stopping
// just before the "--" that introduces the command bwrap itself runs
// (buildArgv adds that part): a read-only /usr with the symlinks real
// binaries expect to find their shared libraries through, a fresh
// /proc, /dev and empty /tmp, spec's own input and output bound at the
// fixed InputPath/OutputDirPath, every namespace unshared, a cleared
// environment with only PATH and LANG set, and this package's own
// seccomp filter.
func bwrapArgs(spec Spec) []string {
	args := []string{
		"--ro-bind", "/usr", "/usr",
		"--symlink", "usr/lib", "/lib",
		"--symlink", "usr/lib", "/lib64",
		"--symlink", "usr/bin", "/bin",
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
	}

	if spec.Input != "" {
		args = append(args, "--ro-bind", spec.Input, InputPath)
	}
	args = append(args, "--bind", spec.OutputDir, OutputDirPath)

	args = append(args,
		"--unshare-all",
		"--die-with-parent",
		"--new-session",
		"--clearenv",
		"--setenv", "PATH", sandboxPath,
		"--setenv", "LANG", sandboxLang,
		"--chdir", workDir,
		"--seccomp", strconv.Itoa(seccompFD),
	)

	return args
}
