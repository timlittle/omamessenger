package sandbox

// seccomp.go builds the seccomp-bpf program every sandboxed job loads:
// a fixed deny list of syscalls a transcode has no legitimate reason
// to call, enforced by the kernel behind bwrap's namespace isolation
// and prlimit's resource caps (see docs/decisions.md for the library
// choice).
//
// Every action returns an errno rather than killing the process:
// killing raises SIGSYS, which core-dumps and triggers a "Process
// crashed" desktop notification even though the sandbox behaved
// correctly. An errno just fails the syscall and lets the process
// exit normally.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"

	seccomp "github.com/elastic/go-seccomp-bpf"
	"github.com/elastic/go-seccomp-bpf/arch"
	"golang.org/x/net/bpf"
)

// archOffset is the byte offset of the arch field in the kernel's
// struct seccomp_data, right after the 4-byte syscall number.
const archOffset = 4

// errnoEPERM and errnoENOSYS are the errno values this filter returns
// instead of killing: EPERM for a denied syscall, ENOSYS for a
// syscall made through the wrong architecture.
const (
	errnoEPERM  = 1
	errnoENOSYS = 38
)

// deniedSyscalls names every syscall no ffmpeg transcode profile has
// any legitimate reason to make: tracing, kernel keyring and BPF
// loading, mount/namespace manipulation, module loading, kexec,
// io_uring, exec personality changes, legacy privileged syscalls, fd
// path/handle tricks, and clock setting. This is defence in depth
// behind bwrap's own isolation; the list only grows from a syscall an
// integration test still passes with it denied, never from a guess.
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

// memfdSyscalls names memfd_create and memfd_secret: both let a
// process allocate shared-memory-backed pages charged to the kernel
// rather than the process's own RLIMIT_AS, so a hijacked job could
// otherwise bypass that limit by holding many of them open. Refusing
// both closes the gap outright.
var memfdSyscalls = []string{"memfd_create", "memfd_secret"}

// ipcSyscalls names the System V shared-memory, message-queue and
// semaphore syscalls plus POSIX message queues: more routes to kernel
// memory not charged against RLIMIT_AS. A fresh IPC namespace
// (--unshare-all) starts at the kernel's own effectively-unlimited
// defaults rather than the host's tuned sysctls, so these get the
// same treatment as memfdSyscalls for the same reason.
var ipcSyscalls = []string{
	"shmget", "shmat", "shmdt", "shmctl",
	"msgget", "msgsnd", "msgrcv", "msgctl",
	"semget", "semop", "semtimedop", "semctl",
	"mq_open", "mq_unlink", "mq_timedsend", "mq_timedreceive", "mq_notify", "mq_getsetattr",
}

// cloneThreadFlag is CLONE_THREAD, clone's flags argument bit that
// tells a new thread (shares the parent's address space) apart from a
// new process (does not).
const cloneThreadFlag = 0x00010000

// processCreationSyscalls names every way to start a new process:
// RLIMIT_AS and RLIMIT_CPU (limits.go) are charged per process, so an
// unrestricted fork would let a hijacked job multiply those bounds
// per child. Threads share the parent's limits instead, so ordinary
// threading must stay allowed; fork and vfork are refused outright,
// while clone and clone3 get their own conditional treatment below.
var processCreationSyscalls = []string{"fork", "vfork"}

// cloneRequiresThread refuses clone with EPERM unless cloneThreadFlag
// is set, allowing thread creation while blocking new processes.
var cloneRequiresThread = seccomp.NameWithConditions{
	Name: "clone",
	Conditions: seccomp.ArgumentConditions{
		{Argument: 0, Operation: seccomp.BitsNotSet, Value: cloneThreadFlag},
	},
}

// clone3Syscalls is refused with ENOSYS rather than EPERM: glibc's
// pthread_create falls back from clone3 to clone only on ENOSYS, so
// EPERM would make thread creation fail outright.
var clone3Syscalls = []string{"clone3"}

// archGate returns the instructions that refuse any syscall made
// through an architecture other than want with ENOSYS, run before the
// rest of the policy: the library's own Policy.Assemble falls through
// to DefaultAction (Allow here) on a foreign architecture, which would
// otherwise bypass the whole denylist, as a 32-bit compat-mode probe
// confirmed on x86_64.
func archGate(want arch.AuditArch) []bpf.Instruction {
	return []bpf.Instruction{
		bpf.LoadAbsolute{Off: archOffset, Size: 4},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(want), SkipTrue: 1},
		bpf.RetConstant{Val: uint32(seccomp.ActionErrno) | errnoENOSYS},
	}
}

// seccompInstructions returns the full program every sandboxed job's
// filter runs: archGate for this build's native architecture, followed
// by the assembled denylist policy.
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
// classic-BPF bytes bwrap's --seccomp FD option expects.
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

// seccompExtraFile returns the read end of a pipe holding the full
// seccomp program, ready to become cmd.ExtraFiles[0] (bwrap's fd 3,
// read via --seccomp 3).
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
		_ = readEnd.Close() // cleanup only; the write error below is what matters
		return nil, fmt.Errorf("sandbox: seccomp: write: %w", err)
	}

	return readEnd, nil
}
