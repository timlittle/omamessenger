package sandbox

// argv.go builds the command line Run executes: nice, ionice, env,
// prlimit, bwrap, then the caller's binary. buildArgv and bwrapArgs
// are pure functions, so they're unit-tested without starting a process.

import (
	"path/filepath"
	"strconv"
)

// seccompFD is the file descriptor bwrap reads its seccomp program from:
// os/exec always assigns ExtraFiles starting at 3, and Run sets exactly
// one entry.
const seccompFD = 3

// ignoredSignals are set to SIG_IGN by env before prlimit, bwrap and the
// sandboxed binary exec: XFSZ (the --fsize limit) and XCPU (the --cpu
// soft-limit warning). SIG_IGN survives exec (POSIX), so this turns a
// limit hit into a failed write or a warning instead of a core-dumping
// kill (see docs/decisions.md).
const ignoredSignals = "XFSZ,XCPU"

// niceLevel is the niceness (nice(1)) applied once on the outermost
// nice call: 19, the lowest an ordinary process can ask for. Niceness
// is inherited across every exec below it (POSIX).
const niceLevel = 19

// ioClass is the I/O scheduling class (ionice(1)) applied once on the
// outermost ionice call: 3, "idle", inherited the same way niceLevel is.
const ioClass = 3

// toolPaths bundles the five external binaries buildArgv chains
// together, each resolved once by Runner.resolveTools.
type toolPaths struct {
	nice, ionice, env, prlimit, bwrap string
}

// buildArgv returns the full command Run starts for spec: nice, ionice,
// env (ignoring ignoredSignals), prlimit, bwrap, then spec.Binary with
// spec.Args.
func buildArgv(spec Spec, tools toolPaths) (name string, args []string) {
	args = []string{
		"-n", strconv.Itoa(niceLevel),
		"--",
		tools.ionice,
		"-c", strconv.Itoa(ioClass),
		"--",
		tools.env,
		"--ignore-signal=" + ignoredSignals,
		"--",
		tools.prlimit,
		"--core=" + strconv.Itoa(maxCoreBytes),
		"--cpu=" + strconv.Itoa(maxCPUSeconds),
		"--as=" + strconv.Itoa(maxAddressSpaceBytes),
		"--fsize=" + strconv.Itoa(maxOutputFileBytes),
		"--nofile=" + strconv.Itoa(maxOpenFiles),
		"--",
		tools.bwrap,
	}
	args = append(args, bwrapArgs(spec)...)
	args = append(args, innerPrlimitArgs(tools.prlimit)...)
	args = append(args, "--", spec.Binary)
	args = append(args, spec.Args...)

	return tools.nice, args
}

// innerPrlimitArgs returns a second prlimit invocation that bwrap execs
// inside the sandbox, right before spec.Binary. RLIMIT_NPROC is counted
// per user ID within whichever user namespace set it, so the outer
// prlimit (run before bwrap creates its own namespace) can't make
// --nproc do anything useful; running it again from inside does.
func innerPrlimitArgs(prlimitPath string) []string {
	return []string{
		"--",
		prlimitPath,
		"--nproc=" + strconv.Itoa(maxProcessCount),
	}
}

// bwrapArgs returns every bwrap flag Run passes for spec: a read-only
// /usr (with the symlinks real binaries expect), a fresh /proc, a
// size-capped /dev/shm and /tmp, spec's input bound read-only, spec's
// single output file bound read-write under OutputDirPath, every
// namespace unshared, a cleared environment, our seccomp filter, and
// finally the root remounted read-only.
//
// --size must precede --tmpfs for /tmp and /dev/shm (bwrap 0.12+), or
// bwrap defaults each to half of system RAM, uncapped by anything above.
//
// The output bind is three flags: mount a fresh tmpfs at OutputDirPath,
// bind spec.Output's single file into it, then remount read-only. That
// keeps the bound file writable while blocking any other write under
// OutputDirPath — previously the whole job directory was bound
// read-write there, which also exposed Input's hard link (see
// docs/decisions.md).
//
// --unshare-user plus --disable-userns close a privilege-escalation
// path --unshare-all alone leaves open: without them a sandboxed job
// can still open a nested user namespace and hold capabilities over it.
//
// --remount-ro / must run last: it is what finally closes the writable
// root tmpfs bwrap starts with, and remounting earlier would break the
// mount setup for /tmp, /dev/shm and the output file above it.
func bwrapArgs(spec Spec) []string {
	args := []string{
		"--ro-bind", "/usr", "/usr",
		"--symlink", "usr/lib", "/lib",
		"--symlink", "usr/lib", "/lib64",
		"--symlink", "usr/bin", "/bin",
		"--proc", "/proc",
		"--dev", "/dev",
		"--size", strconv.Itoa(maxDevShmBytes), "--tmpfs", "/dev/shm",
		"--remount-ro", "/dev",
		"--size", strconv.Itoa(maxTmpBytes), "--tmpfs", "/tmp",
	}

	if spec.Input != "" {
		args = append(args, "--ro-bind", spec.Input, InputPath)
	}

	outputDest := OutputDirPath + "/" + filepath.Base(spec.Output)
	args = append(args,
		"--tmpfs", OutputDirPath,
		"--bind", spec.Output, outputDest,
		"--remount-ro", OutputDirPath,
	)

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
		"--remount-ro", "/",
	)

	return args
}
