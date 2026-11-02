package sandbox

// seccomp.go builds the one seccomp-bpf program every sandboxed job
// loads: a fixed deny list of syscalls a sandboxed transcode has no
// legitimate reason to call, enforced by the kernel itself as one more
// layer behind bwrap's own namespace isolation and prlimit's resource
// caps. See docs/decisions.md for why elastic/go-seccomp-bpf - a pure
// Go, Apache-2.0 library - can build this without cgo, and how the
// result was confirmed to actually stop a denied syscall on this
// machine.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"

	seccomp "github.com/elastic/go-seccomp-bpf"
	"golang.org/x/net/bpf"
)

// deniedSyscalls names every syscall no fixed ffmpeg transcode profile
// this helper runs has any legitimate reason to make: tracing and
// cross-process memory access, the kernel keyring and BPF program
// loading, performance monitoring, userfaultfd, mount and namespace
// manipulation, kernel module loading, kexec, io_uring, and changing
// the process's own exec personality. This is defence in depth behind
// bwrap's own namespace isolation, which already makes most of these
// fail or do nothing useful; a job that reaches one of them is already
// behaving nothing like ffmpeg decoding and encoding a media file, so
// the filter below kills it immediately rather than letting it
// continue in a state nothing here has reasoned about.
var deniedSyscalls = []string{
	"ptrace", "process_vm_readv", "process_vm_writev",
	"keyctl", "add_key", "request_key",
	"bpf", "perf_event_open", "userfaultfd",
	"mount", "umount2", "pivot_root", "unshare", "setns",
	"kexec_load", "kexec_file_load",
	"init_module", "finit_module", "delete_module",
	"io_uring_setup", "io_uring_enter", "io_uring_register",
	"personality",
}

// seccompProgram assembles deniedSyscalls into the raw classic-BPF
// bytes bwrap's own --seccomp FD option expects: the kernel's struct
// sock_filter, packed with no padding, one instruction per 8 bytes -
// exactly what bpf.RawInstruction already is, byte for byte, once
// golang.org/x/net/bpf has assembled it, so no further conversion is
// needed. Every syscall name must resolve on the machine's own
// architecture, or this returns an error rather than loading a filter
// narrower than deniedSyscalls promises.
func seccompProgram() ([]byte, error) {
	policy := seccomp.Policy{
		DefaultAction: seccomp.ActionAllow,
		Syscalls: []seccomp.SyscallGroup{
			{Action: seccomp.ActionKillProcess, Names: deniedSyscalls},
		},
	}

	instructions, err := policy.Assemble()
	if err != nil {
		return nil, fmt.Errorf("sandbox: seccomp: assemble policy: %w", err)
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
