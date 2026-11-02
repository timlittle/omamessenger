package sandbox

// seccomp.go builds the one seccomp-bpf program every sandboxed job
// loads: a fixed deny list of syscalls a sandboxed transcode has no
// legitimate reason to call, enforced by the kernel itself as one more
// layer behind bwrap's own namespace isolation and prlimit's resource
// caps. See docs/decisions.md for why elastic/go-seccomp-bpf - a pure
// Go, Apache-2.0 library - can build this without cgo, and how the
// result was confirmed to actually stop a denied syscall on this
// machine.
//
// Every action here returns an errno rather than killing the process
// (ActionKillProcess, this package's first choice, does not actually
// "kill" quietly: SIGSYS is one of the signals that core-dumps by
// default, and this machine's systemd-coredump turns every one of
// those into a desktop "Process crashed" notification regardless of
// how deep inside a sandbox, or how deliberately, it happened -
// confirmed directly, running this package's own tests). Denying with
// errno avoids that entirely: the syscall simply fails, the process
// keeps running and reports its own ordinary non-zero exit, and
// nothing reaches systemd-coredump at all.

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

// archGate returns the three instructions that run before anything
// else in seccompInstructions: load the syscall's own reported
// architecture, and refuse it with ENOSYS unless it equals want. This
// exists because elastic/go-seccomp-bpf's own Policy.Assemble, when
// the loaded architecture does not match the one the policy was built
// for, jumps straight to the policy's DefaultAction - skipping every
// syscall deniedSyscalls names, allowed or denied. With this
// package's own DefaultAction of Allow, that is a real bypass of the
// whole denylist, not a theoretical one: confirmed on this machine
// with a probe that reached an unshare(CLONE_NEWUSER) the native
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

	policy := seccomp.Policy{
		DefaultAction: seccomp.ActionAllow,
		Syscalls: []seccomp.SyscallGroup{
			{Action: seccomp.Action(uint32(seccomp.ActionErrno) | errnoEPERM), Names: deniedSyscalls},
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
