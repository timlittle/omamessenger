package sandbox

// argv.go builds the exact command line Run executes: nice, lowering
// CPU scheduling priority, running ionice, lowering I/O scheduling
// priority, running env, ignoring the two signals this package's own
// prlimit flags could otherwise trigger, running prlimit, with its
// own resource-limit flags, running bwrap, with its own isolation
// flags, running the caller's binary. buildArgv and bwrapArgs are
// pure functions of their own arguments and the fixed constants in
// limits.go - never of anything a sender or the file being processed
// controls - so they are unit-tested exhaustively without starting
// any process at all (see argv_test.go).

import (
	"path/filepath"
	"strconv"
)

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
// confirmed directly down to the final sandboxed process's own
// /proc/self/status. Ignoring them turns what would otherwise be a
// kill into an ordinary failed write (EFBIG) or a process that keeps
// running past its own soft CPU warning - both of which this
// package's own --core=0 and the hard CPU limit's own unconditional,
// uncatchable SIGKILL (which never core-dumps) still bound. See
// docs/decisions.md for why: every one of these signals core-dumps by
// default, and systemd-coredump records every core-dumping signal,
// with desktop crash notifiers reporting each one as a "Process
// crashed" notification - confirmed directly - however deliberately,
// or deep inside a sandbox, it happened.
const ignoredSignals = "XFSZ,XCPU"

// niceLevel is the niceness (see nice(1)) buildArgv's own outermost
// nice wraps every job in: 19, the least favourable value Linux lets
// an ordinary process ask for itself. A fork or exec always inherits
// the calling process's own niceness (POSIX), confirmed directly all
// the way down to the sandboxed binary bwrap finally execs, so
// setting this once, on nice itself, is enough to cover every exec in
// the chain below it (env, prlimit, bwrap and bwrap's own internal
// fork) without any of them needing to ask for it again. See
// docs/decisions.md for why a job's own CPU priority, not just how
// much CPU time it may use in total, needs bounding too.
const niceLevel = 19

// ioClass is the I/O scheduling class (see ionice(1)) buildArgv's own
// outermost ionice wraps every job in: 3, "idle", the least
// favourable class Linux defines, inherited across fork and exec the
// same way niceLevel is (confirmed directly) - so, like niceLevel,
// setting it once on ionice itself is enough for the whole chain
// below it.
const ioClass = 3

// buildArgv returns the full command Run starts for spec: nice,
// lowering CPU priority to niceLevel, running ionice, lowering I/O
// priority to ioClass, running env, named by its own resolved path,
// ignoring ignoredSignals, running prlimit under every argument
// needed to run bwrap (bwrapPath) under this package's fixed resource
// limits and then, inside it, spec.Binary with spec.Args.
func buildArgv(spec Spec, nicePath, ionicePath, bwrapPath, prlimitPath, envPath string) (name string, args []string) {
	args = []string{
		"-n", strconv.Itoa(niceLevel),
		"--",
		ionicePath,
		"-c", strconv.Itoa(ioClass),
		"--",
		envPath,
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

	return nicePath, args
}

// bwrapArgs returns every bwrap flag Run passes for spec, stopping
// just before the "--" that introduces the command bwrap itself runs
// (buildArgv adds that part): a read-only /usr with the symlinks real
// binaries expect to find their shared libraries through, a fresh
// /proc, /dev with its own /dev/shm capped and separated from it,
// a size-capped empty /tmp, spec's own input bound read-only at the
// fixed InputPath, spec's own single output file bound read-write
// under the fixed OutputDirPath with nothing else reachable there,
// every namespace unshared with no way to open a new user namespace
// from inside, a cleared environment with only PATH and LANG set,
// this package's own seccomp filter, and finally bwrap's own root
// remounted read-only now that everything that needs to stay
// writable already has its own mount.
//
// --size before --tmpfs /tmp (bwrap 0.12+): without it, bwrap's own
// tmpfs defaults to half of this machine's own RAM, counted against
// no limit above (see maxTmpBytes's own doc comment in limits.go).
//
// /dev/shm: --dev /dev already creates it, as part of /dev's own
// tmpfs, with no size of its own - the same uncapped-tmpfs gap
// maxTmpBytes already closes for /tmp, confirmed directly to be just
// as uncapped at this path. Mounting a second, explicitly sized
// tmpfs at /dev/shm right after replaces that; --remount-ro /dev then
// closes /dev itself (nothing in any profile's own fixed ffmpeg
// arguments ever writes there directly) without affecting the
// separate tmpfs now mounted at /dev/shm - confirmed directly through
// a real bwrap, the same way remounting OutputDirPath read-only below
// never affects the file bound inside it.
//
// The output bind is three flags, not one: --tmpfs OutputDirPath
// mounts a fresh, empty, writable tmpfs there first; --bind
// spec.Output's own single file into it, at its own base name, is
// what a profile's fixed ffmpeg arguments actually write to;
// --remount-ro OutputDirPath then remounts that tmpfs read-only,
// which does not affect the file bind already mounted inside it (that
// mount keeps its own, separate read-write flag) but does mean
// nothing else can be created, renamed or removed under OutputDirPath
// at all - confirmed directly, with this exact sequence through a real
// bwrap, that writing to the bound file still works while creating,
// deleting or renaming any other entry under OutputDirPath now fails
// with "Read-only file system". Before this, the whole job directory
// was bound read-write here, which also exposed Input's own hard link
// - living in that same directory - for writing (see
// docs/decisions.md).
//
// --unshare-user plus --disable-userns (on top of --unshare-all,
// which does not by itself stop this): confirmed directly that
// without them, a sandboxed job can still call unshare(CLONE_NEWUSER)
// or clone's own equivalent flag and successfully open a second user
// namespace nested inside the sandbox's own - bwrap's "unshare-all"
// own manual notes this needs an explicit --unshare-user before
// --disable-userns takes effect. A nested user namespace is a
// privilege-regaining trick in its own right (a process can hold
// capabilities, including CAP_SYS_ADMIN, over its own namespace's
// resources), so this closes it rather than leaving it to whatever
// the seccomp filter happens to deny.
//
// --remount-ro / last: bwrap's own root - everywhere not explicitly
// bound to something else - starts out as a plain, writable tmpfs
// with no size of its own, the same uncapped-disk shape as /tmp and
// /dev/shm before their own --size flags above, just with no fixed
// path a caller would ever think to check. Remounting it read-only,
// once every place that still needs to be writable (/tmp, /dev/shm,
// the one output file) already has its own separate mount, closes
// that without touching any of them - confirmed directly through a
// real bwrap, with this exact flag last: a new file anywhere else
// under / fails with "Read-only file system", while /tmp, /dev/shm
// and the bound output file all still accept a write exactly as
// before. This has to run after every other mount in this function,
// never before: remounting first would make bwrap's own later setup
// of those mounts fail outright, not just whatever a job tries to do
// once it starts.
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
