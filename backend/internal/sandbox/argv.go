package sandbox

// argv.go builds the exact command line Run executes: env, ignoring
// the two signals this package's own prlimit flags could otherwise
// trigger, running prlimit, with its own resource-limit flags,
// running bwrap, with its own isolation flags, running the caller's
// binary. buildArgv and bwrapArgs are pure functions of their own
// arguments and the fixed constants in limits.go - never of anything
// a sender or the file being processed controls - so they are
// unit-tested exhaustively without starting any process at all (see
// argv_test.go).

import "strconv"

// seccompFD is the file descriptor bwrap reads its seccomp program
// from, once exec'd: os/exec always gives ExtraFiles its slots
// starting at 3, after the three standard streams, and Run always sets
// exactly one ExtraFiles entry (see process.go), so this is always
// correct, not just the common case.
const seccompFD = 3

// ignoredSignals lists the signals env's own --ignore-signal sets to
// SIG_IGN before prlimit, bwrap and the sandboxed binary each exec in
// turn: XFSZ (prlimit's own --fsize limit) and XCPU (its own --cpu
// limit's soft-limit warning). A signal set to SIG_IGN, unlike a
// caught one, stays SIG_IGN across every exec in the chain (POSIX),
// confirmed directly on this machine down to the final sandboxed
// process's own /proc/self/status. Ignoring them turns what would
// otherwise be a kill into an ordinary failed write (EFBIG) or a
// process that keeps running past its own soft CPU warning - both of
// which this package's own --core=0 and the hard CPU limit's own
// unconditional, uncatchable SIGKILL (which never core-dumps) still
// bound. See docs/decisions.md for why: every one of these signals
// core-dumps by default, and this machine's systemd-coredump turns
// every core dump into a desktop "Process crashed" notification -
// confirmed directly - however deliberately, or deep inside a
// sandbox, it happened.
const ignoredSignals = "XFSZ,XCPU"

// buildArgv returns the full command Run starts for spec: env, named
// by its own resolved path, ignoring ignoredSignals, running prlimit
// under every argument needed to run bwrap (bwrapPath) under this
// package's fixed resource limits and then, inside it, spec.Binary
// with spec.Args.
func buildArgv(spec Spec, bwrapPath, prlimitPath, envPath string) (name string, args []string) {
	args = []string{
		"--ignore-signal=" + ignoredSignals,
		"--",
		prlimitPath,
		"--core=" + strconv.Itoa(maxCoreBytes),
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

	return envPath, args
}

// bwrapArgs returns every bwrap flag Run passes for spec, stopping
// just before the "--" that introduces the command bwrap itself runs
// (buildArgv adds that part): a read-only /usr with the symlinks real
// binaries expect to find their shared libraries through, a fresh
// /proc, /dev and empty /tmp, spec's own input and output bound at the
// fixed InputPath/OutputDirPath, every namespace unshared with no way
// to open a new user namespace from inside, a cleared environment
// with only PATH and LANG set, and this package's own seccomp filter.
//
// --unshare-user plus --disable-userns (on top of --unshare-all,
// which does not by itself stop this): confirmed on this machine that
// without them, a sandboxed job can still call unshare(CLONE_NEWUSER)
// or clone's own equivalent flag and successfully open a second user
// namespace nested inside the sandbox's own - bwrap's "unshare-all"
// own manual notes this needs an explicit --unshare-user before
// --disable-userns takes effect. A nested user namespace is a
// privilege-regaining trick in its own right (a process can hold
// capabilities, including CAP_SYS_ADMIN, over its own namespace's
// resources), so this closes it rather than leaving it to whatever
// the seccomp filter happens to deny.
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
		"--unshare-user",
		"--disable-userns",
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
