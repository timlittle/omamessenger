package sandbox

// limits.go fixes the one set of bounds and sandbox-internal paths
// every job this package runs gets, regardless of caller: how much
// CPU time, address space, output size and open files it may use, and
// what it sees in place of this helper's own filesystem and
// environment. A caller needing a looser or tighter bound is asking
// for a different kind of job than this package is for.

// workDir is the directory every sandboxed command runs in. Bwrap
// creates every missing parent directory a bind target needs, so
// binding InputPath and OutputDirPath underneath it is enough; nothing
// has to create workDir itself first.
const workDir = "/work"

// InputPath is where Run binds Spec.Input, read-only, when it is set:
// the one, fixed location a caller's own Args must name to read its
// input, never a path derived from a sender's own file name.
const InputPath = workDir + "/in"

// OutputDirPath is the directory Run binds Spec.Output's one file
// under, read-write, at that file's own base name: the fixed location
// a caller's own Args must name to write output. Run mounts a fresh,
// empty tmpfs here first, bind-mounts the one file into it, and then
// remounts the tmpfs itself read-only (see bwrapArgs): the file stays
// writable - that one mount keeps its own read-write flag regardless
// of what happens to the directory around it - but nothing else is
// ever creatable, renameable or removable under this path, so a job
// can reach no file here but the one its caller actually bound.
const OutputDirPath = workDir + "/out"

// maxTmpBytes bounds the empty tmpfs Run mounts at /tmp (bwrap's own
// --size, read just before --tmpfs; see argv.go): none of this
// helper's own fixed ffmpeg arguments ever ask it to write anything
// there, so this only has to cover whatever a linked library's own
// startup might still touch - never real output, which OutputDirPath
// bounds instead. Without a --size, bwrap's own tmpfs defaults to half
// of this machine's own RAM, counted against no limit above it: a
// hijacked job could fill that much disk (tmpfs pages are reclaimed
// from, and swap back to, real storage under memory pressure) well
// within its own wall-clock timeout. 16 MiB is generous for ordinary
// startup use while bounding exactly that.
const maxTmpBytes = 16 << 20

// InputBaseName is the only base name Spec.validate accepts for
// Spec.Input: a caller places its own file under this name, inside a
// directory it controls, before ever building a Spec (transcode's
// own job directory does exactly that; see transcode.placeJobInput).
// The literal path given as Spec.Input - never InputPath, the fixed
// location inside the sandbox - reaches bwrap's own argv, which is
// readable by any local user through /proc for the life of the job
// (see the package doc comment), so it must never carry a sender's
// file name or a user-picked path of their own choosing.
const InputBaseName = "input"

// sandboxPath is the only PATH a sandboxed command sees, set by
// --clearenv plus --setenv rather than inherited from this helper's
// own environment. /usr/bin and /bin (itself a symlink to usr/bin, see
// bwrapArgs) are the only directories bound into the sandbox at all,
// so nothing wider could ever resolve regardless.
const sandboxPath = "/usr/bin:/bin"

// sandboxLang fixes the one locale a sandboxed command sees, rather
// than whatever this helper's own process happens to have inherited:
// the plain C locale with UTF-8 encoding, so a command's own text
// output is always in a predictable, parseable form.
const sandboxLang = "C.UTF-8"

// maxCoreBytes bounds how large a core dump a job's own process may
// write (prlimit --core), and is 0: no core data ever saved. This is
// not, on its own, enough to stop a crash notification reaching the
// desktop - confirmed directly: even with this at 0, a process the
// kernel still delivers a core-dumping signal to still makes
// systemd-coredump log the event (COREFILE "none" rather than
// "present"), and desktop crash notifiers still report "Process
// crashed" for that log entry regardless of whether any data was
// saved. Avoiding the signal in the first place is what actually
// prevents the notification: env's own --ignore-signal (see argv.go
// and docs/decisions.md) does that for prlimit's own --fsize and the
// soft half of its own --cpu limit, and the seccomp filter returns an
// errno rather than killing (see seccomp.go's own package doc
// comment) for every syscall it denies. What this limit still bounds:
// the hard half of --cpu, which the kernel enforces with an
// unconditional, uncatchable SIGKILL - not core-dumping by default,
// so nothing to suppress there - and a genuine crash in ffmpeg
// itself (SIGSEGV, SIGABRT and the like), which is the one case
// nothing here can prevent from reaching the desktop, only keep from
// also leaving a core file nothing here needs.
const maxCoreBytes = 0

// maxCPUSeconds bounds a job's total CPU time across every thread it
// starts (prlimit --cpu): the kernel charges RLIMIT_CPU to a whole
// thread group's combined usage, not to each thread separately, and
// now actually bounds the whole job, not just its first thread -
// seccomp.go's own process-creation rules refuse every way a job could
// otherwise split into a second process with a fresh limit of its
// own, leaving threading (which shares this same total) as the only
// way left to run anything concurrently at all. transcode's own
// longest-running profile bounds its own wall-clock timeout to 5
// minutes at two threads, which at full use of both could reach 600
// CPU-seconds; this is 1.5x that, so it should never actually fire
// before the caller's own context deadline does - it exists only to
// bound a process still running after that.
const maxCPUSeconds = 900

// maxAddressSpaceBytes bounds a job's total virtual memory (prlimit
// --as): shared by every thread a job starts, the same as
// maxCPUSeconds above, and for the same reason now actually a bound on
// the whole job rather than just whichever process happens to hold it
// - a thread cannot have its own address space, by definition, so
// there is no way left to dodge this one either once seccomp.go's own
// rules close every way to fork. Arch Linux's ffmpeg package links a
// large number of optional libraries (CUDA, Vulkan, VapourSynth and
// others, confirmed from its own -version output) that the dynamic
// linker maps into the process before a single frame is read, well
// before any real encode buffer is requested; measured directly,
// re-encoding a 720x1280 clip with audio at two threads - transcode's
// own Video profile - failed inside libx264's own allocator under a
// 2 GiB cap and succeeded under 3 GiB, with or without bwrap, so the
// cap is not bwrap overhead but this ffmpeg build's own baseline
// footprint. This is 4 GiB: a full GiB of headroom above the measured
// 3 GiB success point, generous for every profile transcode runs
// while still bounding what a crafted file's declared dimensions or
// bitrate could make an encoder try to allocate beyond that baseline
// (see docs/decisions.md).
const maxAddressSpaceBytes = 4 << 30

// maxOutputFileBytes bounds how large any single file a job writes
// may grow (prlimit --fsize) before the kernel refuses any further
// write to it with EFBIG, rather than killing the job outright: env's
// own --ignore-signal (see argv.go) makes that so, in place of the
// SIGXFSZ the kernel would otherwise deliver, so a job hitting this
// limit fails on an ordinary write error, which outputReachedSizeLimit
// then recognises from the file it leaves behind (see
// Result.TooLarge). A backstop well above what a legitimate job run
// through this package should ever write, so it exists to bound a
// pathological or hostile case, not to police an ordinary one - a
// caller wanting a tighter, profile-specific cap checks the file it
// gets back itself.
const maxOutputFileBytes = 512 << 20

// maxOpenFiles bounds how many files a job may hold open at once
// (prlimit --nofile): modest, since every profile this helper runs
// opens at most a handful of files (its one input, its one output,
// and whatever small number of library files ffmpeg itself opens),
// confirmed directly (see docs/decisions.md).
const maxOpenFiles = 64

// maxCapturedStderr bounds how much of a job's own stderr Run keeps
// in memory (see boundedStderr): enough to tell a handful of known
// failure shapes apart (see classify.go), far less than a pathological
// or hostile command could otherwise write.
const maxCapturedStderr = 4 << 10
