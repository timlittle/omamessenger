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

// OutputDirPath is where Run binds Spec.OutputDir, read-write: the
// one, fixed location a caller's own Args must name to write output.
const OutputDirPath = workDir + "/out"

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

// maxCPUSeconds bounds a job's total CPU time across every thread it
// starts (prlimit --cpu). transcode's own longest-running profile
// bounds its own wall-clock timeout to 5 minutes at two threads, which
// at full use of both could reach 600 CPU-seconds; this is 1.5x that,
// so it should never actually fire before the caller's own context
// deadline does - it exists only to bound a process still running
// after that.
const maxCPUSeconds = 900

// maxAddressSpaceBytes bounds a job's total virtual memory (prlimit
// --as). This machine's own ffmpeg links a large number of optional
// libraries (CUDA, Vulkan, VapourSynth and others, confirmed from its
// own -version output) that the dynamic linker maps into the process
// before a single frame is read, well before any real encode buffer
// is requested; measured directly on this machine, re-encoding a
// 720x1280 clip with audio at two threads - transcode's own Video
// profile - failed inside libx264's own allocator under a 2 GiB cap
// and succeeded under 3 GiB, with or without bwrap, so the cap is not
// bwrap overhead but this ffmpeg build's own baseline footprint. This
// is 4 GiB: a full GiB of headroom above the measured 3 GiB success
// point, generous for every profile transcode runs while still
// bounding what a crafted file's declared dimensions or bitrate could
// make an encoder try to allocate beyond that baseline (see
// docs/decisions.md).
const maxAddressSpaceBytes = 4 << 30

// maxOutputFileBytes bounds how large any single file a job writes
// may grow (prlimit --fsize) before the kernel kills it with SIGXFSZ
// (see classifyWaitErr and Result.TooLarge): a backstop well above
// what a legitimate job run through this package should ever write,
// so it exists to bound a pathological or hostile case, not to police
// an ordinary one - a caller wanting a tighter, profile-specific cap
// checks the file it gets back itself.
const maxOutputFileBytes = 512 << 20

// maxOpenFiles bounds how many files a job may hold open at once
// (prlimit --nofile): modest, since every profile this helper runs
// opens at most a handful of files (its one input, its one output,
// and whatever small number of library files ffmpeg itself opens),
// confirmed on this machine (see docs/decisions.md).
const maxOpenFiles = 64

// maxCapturedStderr bounds how much of a job's own stderr Run keeps
// in memory (see boundedStderr): enough to tell a handful of known
// failure shapes apart (see classify.go), far less than a pathological
// or hostile command could otherwise write.
const maxCapturedStderr = 4 << 10
