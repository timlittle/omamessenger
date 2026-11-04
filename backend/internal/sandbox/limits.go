package sandbox

// limits.go fixes the one set of resource bounds and sandbox-internal
// paths every job this package runs gets, regardless of caller.

// workDir is the directory every sandboxed command runs in. Bwrap
// creates missing parent directories for bind targets automatically.
const workDir = "/work"

// InputPath is where Run binds Spec.Input read-only when set: the one
// fixed location a caller's Args must name, never a sender-derived path.
const InputPath = workDir + "/in"

// OutputDirPath is the directory Run binds Spec.Output's file under,
// read-write, at that file's base name. The surrounding tmpfs is
// remounted read-only after the bind (see bwrapArgs), so the job can
// reach no other file here.
const OutputDirPath = workDir + "/out"

// maxTmpBytes bounds the tmpfs Run mounts at /tmp: without a --size,
// bwrap defaults to half of system RAM, which a hijacked job could
// fill as disk. 16 MiB covers ordinary startup use only; real output
// goes through OutputDirPath instead.
const maxTmpBytes = 16 << 20

// maxDevShmBytes bounds the tmpfs at /dev/shm the same way maxTmpBytes
// bounds /tmp: bwrap's --dev otherwise leaves it uncapped at half of
// system RAM.
const maxDevShmBytes = 16 << 20

// InputBaseName is the only base name Spec.validate accepts for
// Spec.Input. It reaches bwrap's argv, which any local user can read
// via /proc for the job's lifetime, so it must never carry a sender's
// file name.
const InputBaseName = "input"

// sandboxPath is the only PATH a sandboxed command sees, set via
// --clearenv plus --setenv rather than inherited.
const sandboxPath = "/usr/bin:/bin"

// sandboxLang fixes the locale a sandboxed command sees to plain C
// with UTF-8, so a command's text output is always predictable to parse.
const sandboxLang = "C.UTF-8"

// maxCoreBytes (prlimit --core) is 0: no core dump data is ever saved.
// On its own this doesn't stop a crash notification reaching the
// desktop; avoiding the triggering signal (env's --ignore-signal,
// argv.go) is what does that. This still bounds genuine crashes
// (SIGSEGV etc.) and the hard CPU limit's SIGKILL.
const maxCoreBytes = 0

// maxCPUSeconds (prlimit --cpu) bounds a job's total CPU time across
// every thread, since the kernel charges RLIMIT_CPU to the whole
// thread group. 900 is 1.5x the worst case of the longest profile's
// wall-clock timeout, so it should never fire before the context
// deadline does.
const maxCPUSeconds = 900

// maxAddressSpaceBytes (prlimit --as) bounds a job's total virtual
// memory, shared across every thread it starts. Measured directly:
// Arch's ffmpeg build (many optional linked libraries) needs about
// 3 GiB just to encode transcode's Video profile; 4 GiB leaves a GiB
// of headroom above that baseline (see docs/decisions.md).
const maxAddressSpaceBytes = 4 << 30

// maxOutputFileBytes (prlimit --fsize) bounds how large any file a job
// writes may grow before the kernel refuses further writes with EFBIG
// (env's --ignore-signal turns what would be a kill into that ordinary
// error). A backstop far above any legitimate output, not a
// profile-specific cap.
const maxOutputFileBytes = 512 << 20

// maxOpenFiles (prlimit --nofile) is modest: every profile opens at
// most a handful of files.
const maxOpenFiles = 64

// maxProcessCount (prlimit --nproc, run inside the sandbox via
// innerPrlimitArgs, not the outer prlimit) bounds tasks held by the
// job's user ID. RLIMIT_NPROC is counted per user ID within the
// namespace that set it, so the outer prlimit (run before bwrap's own
// namespace exists) can't bound it; seccomp still allows
// clone(CLONE_THREAD) for the "-threads 2" profiles need, so this
// stops an unbounded thread-spawn loop instead. 40 is more than double
// the heaviest profile's measured peak of 17 threads (see
// docs/decisions.md).
const maxProcessCount = 40

// maxCapturedStderr bounds how much of a job's stderr Run keeps in
// memory: enough to tell known failure shapes apart (classify.go).
const maxCapturedStderr = 4 << 10

// maxJobBytes is the total memory and disk one job can reach, summing
// every writable place inside the sandbox: maxAddressSpaceBytes,
// maxTmpBytes, maxDevShmBytes and maxOutputFileBytes. Kept as a sum
// rather than a literal so a future change to any one of them changes
// this and its pinning test together. memfd and System V/POSIX IPC
// are refused outright by seccomp rather than added here, since
// neither has a cap that would make a number meaningful.
const maxJobBytes = maxAddressSpaceBytes + maxTmpBytes + maxDevShmBytes + maxOutputFileBytes
