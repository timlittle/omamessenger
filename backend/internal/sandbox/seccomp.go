package sandbox

// seccomp.go builds the one seccomp-bpf program every sandboxed job
// loads: a fixed deny list of syscalls a sandboxed transcode has no
// legitimate reason to call, enforced by the kernel itself as one more
// layer behind bwrap's own namespace isolation and prlimit's resource
// caps. See docs/decisions.md for why elastic/go-seccomp-bpf - a pure
// Go, Apache-2.0 library - can build this without cgo, and how the
// result was confirmed to actually stop a denied syscall.
//
// Every action here returns an errno rather than killing the process
// (ActionKillProcess, this package's first choice, does not actually
// "kill" quietly: SIGSYS is one of the signals that core-dumps by
// default, and systemd-coredump records every one of those, with
// desktop crash notifiers reporting each as a "Process crashed"
// notification regardless of how deep inside a sandbox, or how
// deliberately, it happened - confirmed directly, running this
// package's own tests). Denying with errno avoids that entirely: the
// syscall simply fails, the process keeps running and reports its own
// ordinary non-zero exit, and nothing reaches systemd-coredump at
// all.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"

	seccomp "github.com/elastic/go-seccomp-bpf"
	"github.com/elastic/go-seccomp-bpf/arch"
	"golang.org/x/net/bpf"
)

// archOffset is the byte offset of the arch field within the kernel's
// own struct seccomp_data ({ int nr; __u32 arch; __u64
// instruction_pointer; __u64 args[6]; }), right after the 4-byte
// syscall number: part of the stable kernel UAPI, not something
// either library this package uses exports.
const archOffset = 4

// errnoEPERM and errnoENOSYS are the Linux errno values this
// package's own filter returns instead of killing: EPERM (1) for a
// denied syscall, ENOSYS (38) for a syscall made through the wrong
// architecture (mirroring the library's own x32Filter, which already
// uses ENOSYS for its own "wrong ABI" case). Both are stable across
// every Linux architecture, not something either library this
// package uses exports as a constant.
const (
	errnoEPERM  = 1
	errnoENOSYS = 38
)

// deniedSyscalls names every syscall no fixed ffmpeg transcode profile
// this helper runs has any legitimate reason to make: tracing and
// cross-process memory access, the kernel keyring and BPF program
// loading, performance monitoring, userfaultfd, mount and namespace
// manipulation (the older mount/umount2 calls and the newer fsopen-
// family API alike), kernel module loading, kexec, io_uring, changing
// the process's own exec personality, legacy and privileged-only
// system calls (modify_ldt, uselib, syslog, acct, quotactl, swapon/
// off, reboot, iopl, ioperm, chroot, fanotify_init), ways to turn a
// file descriptor back into a path or walk one by an opaque handle
// (kcmp, name_to_handle_at, open_by_handle_at, lookup_dcookie), and
// setting the system clock. This is defence in depth behind bwrap's
// own namespace isolation, which already makes most of these fail or
// do nothing useful; a job that reaches one of them is already
// behaving nothing like ffmpeg decoding and encoding a media file, so
// the filter below refuses it with EPERM immediately rather than
// letting it continue in a state nothing here has reasoned about (see
// the package doc comment for why EPERM, not a kill). Deliberately
// absent: anything NUMA-related, or that glibc or one of ffmpeg's own
// linked libraries might call during ordinary startup - this list
// only grows from a syscall this helper's own real integration tests
// (see docs/decisions.md) still pass with denied, never from a guess.
var deniedSyscalls = []string{
	"ptrace", "process_vm_readv", "process_vm_writev",
	"keyctl", "add_key", "request_key",
	"bpf", "perf_event_open", "userfaultfd",
	"mount", "umount2", "pivot_root", "unshare", "setns",
	"fsopen", "fsconfig", "fsmount", "fspick", "move_mount", "open_tree", "mount_setattr",
	"kexec_load", "kexec_file_load",
	"init_module", "finit_module", "delete_module",
	"io_uring_setup", "io_uring_enter", "io_uring_register",
	"personality",
	"modify_ldt", "uselib", "syslog", "acct", "quotactl",
	"swapon", "swapoff", "reboot", "iopl", "ioperm", "chroot", "fanotify_init",
	"kcmp", "name_to_handle_at", "open_by_handle_at", "lookup_dcookie",
	"clock_settime", "clock_adjtime", "settimeofday", "adjtimex",
}

// memfdSyscalls names the two ways to create an anonymous,
// shared-memory-backed file that has its own fd but no file-size
// limit of its own: memfd_create (ordinary anonymous memory) and
// memfd_secret (kernel-locked memory meant never to reach disk or be
// read by anything but the process that created it, not even its own
// parent). Both differ from every entry in deniedSyscalls in exactly
// the way that matters here: the pages behind the fd they return are
// charged to the kernel's own shared-memory accounting, never to the
// calling process's own virtual address space, so maxAddressSpaceBytes
// - RLIMIT_AS, enforced per process - never sees them written to at
// all, only mapped. What does still apply to a memfd is RLIMIT_FSIZE
// (prlimit's own --fsize) and RLIMIT_NOFILE (--nofile): before this
// fix, a hijacked job could hold up to maxOpenFiles of them open at
// once, each written up to maxOutputFileBytes, for close to 32 GiB
// through a route no other limit in this package actually bounds.
// Refusing both outright closes that to zero, rather than leaving it
// as one more thing for limits.go's own numbers to somehow cover.
var memfdSyscalls = []string{"memfd_create", "memfd_secret"}

// ipcSyscalls names every System V shared-memory, System V message-
// queue, System V semaphore and POSIX message-queue syscall: three
// more routes, alongside memfd, to kernel memory charged to no
// process's own RLIMIT_AS at all once the backing segment is
// attached, written and detached again. docs/decisions.md's own third
// sandbox review considered shmget/shmat and left them allowed,
// reasoning they were "bounded differently - by system-wide shmmax/
// shmall sysctls". That reasoning does not hold inside this sandbox:
// --unshare-all (argv.go) gives every job a fresh IPC namespace, which
// starts at the kernel's own compiled-in defaults - confirmed directly
// on this machine, where /proc/sys/kernel/shmall reads
// 18446744073692774399, effectively unlimited - never at whatever this
// helper's own host happens to have tuned those sysctls to. A hijacked
// ffmpeg can loop shmget/shmat/fill/shmdt: the pages stay allocated,
// charged to the kernel's own shared-memory accounting rather than any
// process's address space, until the job's whole IPC namespace is torn
// down - up to videoTimeout's own 5 minutes - during which the OOM
// killer, unable to free memory that belongs to no process, starts
// killing real processes instead, such as omarchy-shell itself. Message
// queues (msgget and friends) and semaphore sets (semget and friends)
// share the same per-namespace-default, no-RLIMIT_AS-coverage shape,
// so they are refused for the same reason even though no profile this
// package runs has ever been seen to call them; mq_open and its own
// POSIX message-queue siblings are a fourth, independent API to the
// same kind of kernel object (backed by a mounted mqueue filesystem,
// not SysV's own key-based tables) and get the same treatment. None of
// deniedSyscalls' own "no legitimate reason to call this" framing
// applies here - the reason is the RLIMIT_AS bypass memfdSyscalls
// already exists to close, just three more ways into it - so this gets
// its own named group and doc comment, the same way memfdSyscalls did.
var ipcSyscalls = []string{
	"shmget", "shmat", "shmdt", "shmctl",
	"msgget", "msgsnd", "msgrcv", "msgctl",
	"semget", "semop", "semtimedop", "semctl",
	"mq_open", "mq_unlink", "mq_timedsend", "mq_timedreceive", "mq_notify", "mq_getsetattr",
}

// cloneThreadFlag is CLONE_THREAD, from the kernel's own
// include/uapi/linux/sched.h: part of the stable UAPI, not something
// either library this package uses exports as a constant. It is
// clone's own first argument (its flags, arg0 on every architecture
// this helper ships for) telling apart its two, otherwise identical,
// uses: starting a new thread that shares this process's address
// space (set - glibc's own pthread_create always sets it, confirmed
// directly against this build's own glibc) or starting an entirely
// separate process with its own (clear).
const cloneThreadFlag = 0x00010000

// processCreationSyscalls names every way this helper's own glibc and
// kernel let a process start another one, refused below for a reason
// deniedSyscalls' own entries are not: sandbox/limits.go's RLIMIT_AS
// and RLIMIT_CPU are both limits the kernel charges to a process (for
// a multithreaded one, to its whole thread group's combined total),
// never to a tree of them, so a hijacked ffmpeg that could fork would
// give each child its own fresh 4 GiB and 900 CPU-seconds, multiplying
// every bound those limits promise by however many children it
// started. A new thread has no such gap - it shares its parent's
// address space outright and is charged against that same combined
// CPU total - so refusing every way to start a process while still
// allowing ordinary threading (which every profile's own "-threads 2"
// needs) is what actually makes limits.go's own claim true: a bound on
// the whole job, not just its first process.
//
// fork and vfork can never be a thread - both create a new process by
// definition - so both are refused outright here, the same as
// deniedSyscalls' own entries; clone is instead checked against
// cloneThreadFlag (see cloneRequiresThread) and clone3 against its own
// ENOSYS exception (see clone3Syscalls), since neither can be refused
// the same unconditional way without also refusing the threading every
// profile needs.
var processCreationSyscalls = []string{"fork", "vfork"}

// cloneRequiresThread is clone's own entry in the filter below: EPERM
// unless cloneThreadFlag is set in its own flags argument (see
// processCreationSyscalls' own doc comment for why threads are the
// one process-creation call this filter still allows). BitsNotSet
// matches when the given bits are clear, so this matches - and so
// refuses - exactly the calls that do not set cloneThreadFlag.
var cloneRequiresThread = seccomp.NameWithConditions{
	Name: "clone",
	Conditions: seccomp.ArgumentConditions{
		{Argument: 0, Operation: seccomp.BitsNotSet, Value: cloneThreadFlag},
	},
}

// clone3Syscalls is refused with ENOSYS, not EPERM: the one deliberate
// exception to this file's own "EPERM for denied, ENOSYS only for a
// foreign architecture" rule (see the package doc comment). glibc's
// own pthread_create tries clone3 first and falls back to the clone
// call cloneRequiresThread covers on exactly ENOSYS, never on EPERM -
// confirmed directly, on this build's own glibc (2.44): a filter
// refusing clone3 with EPERM instead makes pthread_create fail
// outright, while ENOSYS lets it fall back to clone and succeed.
// glibc added the fallback for a kernel genuinely too old to have
// clone3 at all, which is exactly what ENOSYS already means, so EPERM
// never reaches the code path that would retry.
var clone3Syscalls = []string{"clone3"}

// archGate returns the three instructions that run before anything
// else in seccompInstructions: load the syscall's own reported
// architecture, and refuse it with ENOSYS unless it equals want. This
// exists because elastic/go-seccomp-bpf's own Policy.Assemble, when
// the loaded architecture does not match the one the policy was built
// for, jumps straight to the policy's DefaultAction - skipping every
// syscall deniedSyscalls names, allowed or denied. With this
// package's own DefaultAction of Allow, that is a real bypass of the
// whole denylist, not a theoretical one: confirmed directly with a
// probe that reached an unshare(CLONE_NEWUSER) the native
// x86_64 path already refuses, by making the identical call through
// the i386 ABI (`int 0x80`, reachable whenever the kernel is built
// with CONFIG_IA32_EMULATION) instead. Running this gate first, and
// refusing anything but want, means the library's own program - and
// its own bypass on a foreign architecture - is never reached by any
// architecture this gate itself did not already let through.
//
// ENOSYS, not a kill: see the package doc comment for why every
// action here returns an errno rather than killing the process.
func archGate(want arch.AuditArch) []bpf.Instruction {
	return []bpf.Instruction{
		bpf.LoadAbsolute{Off: archOffset, Size: 4},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(want), SkipTrue: 1},
		bpf.RetConstant{Val: uint32(seccomp.ActionErrno) | errnoENOSYS},
	}
}

// seccompInstructions returns the full program every sandboxed job's
// seccomp filter runs, as the instruction types golang.org/x/net/bpf
// itself understands rather than the raw bytes a real kernel consumes
// (see seccompProgram): archGate, resolved for this build's own
// native architecture - x86_64 or arm64, whichever this binary was
// compiled for, the same resolution elastic/go-seccomp-bpf's own
// Policy.Assemble does internally - followed by its own assembled
// policy. Gating on this build's own native architecture, rather
// than x86_64 specifically, closes the same bypass on an arm64
// release too: arm64's own 32-bit compat mode (CONFIG_COMPAT) is the
// exact same shape of hole the probe above found on x86_64, under a
// different architecture pair.
func seccompInstructions() ([]bpf.Instruction, error) {
	native, err := arch.GetInfo("")
	if err != nil {
		return nil, fmt.Errorf("sandbox: seccomp: %w", err)
	}

	epermAction := seccomp.Action(uint32(seccomp.ActionErrno) | errnoEPERM)
	policy := seccomp.Policy{
		DefaultAction: seccomp.ActionAllow,
		Syscalls: []seccomp.SyscallGroup{
			{Action: epermAction, Names: deniedSyscalls},
			{Action: epermAction, Names: memfdSyscalls},
			{Action: epermAction, Names: ipcSyscalls},
			{
				Action:             epermAction,
				Names:              processCreationSyscalls,
				NamesWithCondtions: []seccomp.NameWithConditions{cloneRequiresThread},
			},
			{Action: seccomp.Action(uint32(seccomp.ActionErrno) | errnoENOSYS), Names: clone3Syscalls},
		},
	}

	policyInstructions, err := policy.Assemble()
	if err != nil {
		return nil, fmt.Errorf("sandbox: seccomp: assemble policy: %w", err)
	}

	return append(archGate(native.ID), policyInstructions...), nil
}

// seccompProgram assembles seccompInstructions into the raw
// classic-BPF bytes bwrap's own --seccomp FD option expects: the
// kernel's struct sock_filter, packed with no padding, one
// instruction per 8 bytes - exactly what bpf.RawInstruction already
// is, byte for byte, once golang.org/x/net/bpf has assembled it, so
// no further conversion is needed. Every syscall name must resolve on
// the machine's own architecture, or this returns an error rather
// than loading a filter narrower than deniedSyscalls promises.
func seccompProgram() ([]byte, error) {
	instructions, err := seccompInstructions()
	if err != nil {
		return nil, err
	}

	raw, err := bpf.Assemble(instructions)
	if err != nil {
		return nil, fmt.Errorf("sandbox: seccomp: assemble bpf: %w", err)
	}

	buf := &bytes.Buffer{}
	for _, instr := range raw {
		if err := binary.Write(buf, binary.LittleEndian, instr); err != nil {
			return nil, fmt.Errorf("sandbox: seccomp: encode: %w", err)
		}
	}

	return buf.Bytes(), nil
}

// seccompExtraFile returns the read end of a pipe already holding the
// full seccomp program (see seccompProgram), ready to become
// cmd.ExtraFiles[0] - bwrap's own fd 3 once exec'd, read through
// --seccomp 3 (see bwrapArgs). The write end is only ever held here,
// written to once and closed before this returns: deniedSyscalls
// assembles to well under a pipe's default 64 KiB buffer, so the
// write never blocks, and closing the only write end immediately after
// means the kernel signals EOF to the one remaining reader - bwrap
// itself - as soon as it has read every byte.
func seccompExtraFile() (*os.File, error) {
	program, err := seccompProgram()
	if err != nil {
		return nil, err
	}

	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("sandbox: seccomp: pipe: %w", err)
	}
	defer writeEnd.Close()

	if _, err := writeEnd.Write(program); err != nil {
		_ = readEnd.Close() // cleanup only; the write error below is what actually matters here
		return nil, fmt.Errorf("sandbox: seccomp: write: %w", err)
	}

	return readEnd, nil
}
