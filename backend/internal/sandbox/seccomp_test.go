package sandbox

// seccomp_test.go tests seccompProgram and seccompExtraFile directly,
// as an internal test: both are unexported, and assembling the
// program itself needs no real bwrap, so it is worth proving on its
// own rather than only through the slower, skip-if-unavailable real
// enforcement test in sandbox_test.go (TestRun_SeccompDeniesUnshare).

import (
	"encoding/binary"
	"io"
	"testing"

	seccomp "github.com/elastic/go-seccomp-bpf"
	"github.com/elastic/go-seccomp-bpf/arch"
	"golang.org/x/net/bpf"
)

func TestSeccompProgram(t *testing.T) {
	t.Parallel()

	program, err := seccompProgram()
	if err != nil {
		t.Fatalf("seccompProgram() error = %v", err)
	}

	if len(program) == 0 {
		t.Fatal("seccompProgram() returned no bytes")
	}
	if len(program)%8 != 0 {
		t.Fatalf("seccompProgram() returned %d bytes, want a multiple of 8 (struct sock_filter is 8 bytes)", len(program))
	}

	again, err := seccompProgram()
	if err != nil {
		t.Fatalf("second seccompProgram() error = %v", err)
	}
	if string(program) != string(again) {
		t.Error("seccompProgram() is not deterministic across calls")
	}
}

func TestSeccompExtraFile(t *testing.T) {
	t.Parallel()

	want, err := seccompProgram()
	if err != nil {
		t.Fatalf("seccompProgram() error = %v", err)
	}

	f, err := seccompExtraFile()
	if err != nil {
		t.Fatalf("seccompExtraFile() error = %v", err)
	}
	defer f.Close()

	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("reading seccompExtraFile's own fd: %v", err)
	}

	if string(got) != string(want) {
		t.Errorf("seccompExtraFile() holds %d bytes, want the %d bytes seccompProgram() returns", len(got), len(want))
	}
}

// seccompDataPacket builds a struct seccomp_data-shaped buffer for
// golang.org/x/net/bpf's own software VM: nr and archID in the first
// 8 bytes, instruction_pointer and args left zero since this
// package's filter never reads them. The VM's own LoadAbsolute always
// reads multi-byte fields big-endian (vm_instructions.go's loadCommon
// - the same convention real network-packet BPF uses), which is NOT
// how the real kernel lays out seccomp_data (native/little-endian on
// x86_64 and arm64): this layout is only for driving the VM in a test,
// never written anywhere a real kernel reads from.
func seccompDataPacket(nr int32, archID uint32) []byte {
	buf := make([]byte, 64) // sizeof(struct seccomp_data)
	buf[0] = byte(nr >> 24)
	buf[1] = byte(nr >> 16)
	buf[2] = byte(nr >> 8)
	buf[3] = byte(nr)
	buf[4] = byte(archID >> 24)
	buf[5] = byte(archID >> 16)
	buf[6] = byte(archID >> 8)
	buf[7] = byte(archID)

	return buf
}

// runSeccompVM assembles seccompInstructions and runs pkt through
// golang.org/x/net/bpf's own VM, returning the action it computed.
func runSeccompVM(t *testing.T, pkt []byte) uint32 {
	t.Helper()

	instructions, err := seccompInstructions()
	if err != nil {
		t.Fatalf("seccompInstructions() error = %v", err)
	}

	vm, err := bpf.NewVM(instructions)
	if err != nil {
		t.Fatalf("bpf.NewVM() error = %v", err)
	}

	action, err := vm.Run(pkt)
	if err != nil {
		t.Fatalf("vm.Run() error = %v", err)
	}

	return uint32(action)
}

func TestSeccompInstructions_RefusesAForeignArchitectureRegardlessOfSyscall(t *testing.T) {
	t.Parallel()

	// i386 is the exact bypass a security review found: elastic/go-
	// seccomp-bpf's own Policy.Assemble jumps straight to its
	// DefaultAction whenever the loaded arch does not match the one
	// the policy was built for, skipping every syscall it names -
	// confirmed directly by a probe that reached an
	// unshare(CLONE_NEWUSER) the native x86_64 path already refuses,
	// by making the identical call through the i386 ABI (int 0x80)
	// instead. The gate this package adds in front of the library's
	// own program must refuse on sight, before any syscall-specific
	// check ever runs - this uses syscall 0 (read on x86_64) to prove
	// that: an always-allowed syscall number must still be refused
	// here purely because the architecture itself is wrong.
	pkt := seccompDataPacket(0, uint32(arch.I386.ID))

	want := uint32(seccomp.ActionErrno) | errnoENOSYS
	if got := runSeccompVM(t, pkt); got != want {
		t.Errorf("action = %#x, want ActionErrno|ENOSYS (%#x)", got, want)
	}
}

func TestSeccompInstructions_RefusesEveryDeniedSyscallOnTheNativeArch(t *testing.T) {
	t.Parallel()

	native, err := arch.GetInfo("")
	if err != nil {
		t.Fatalf("arch.GetInfo() error = %v", err)
	}

	want := uint32(seccomp.ActionErrno) | errnoEPERM

	for _, name := range deniedSyscalls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			nr, ok := native.SyscallNames[name]
			if !ok {
				t.Fatalf("native arch %s has no syscall number for %q", native.Name, name)
			}

			pkt := seccompDataPacket(int32(nr), uint32(native.ID))
			if got := runSeccompVM(t, pkt); got != want {
				t.Errorf("action for %s (nr=%d) = %#x, want ActionErrno|EPERM (%#x)", name, nr, got, want)
			}
		})
	}
}

func TestSeccompInstructions_AllowsOrdinarySyscallsOnTheNativeArch(t *testing.T) {
	t.Parallel()

	native, err := arch.GetInfo("")
	if err != nil {
		t.Fatalf("arch.GetInfo() error = %v", err)
	}

	for _, name := range []string{"read", "write", "mmap"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			nr, ok := native.SyscallNames[name]
			if !ok {
				t.Fatalf("native arch %s has no syscall number for %q", native.Name, name)
			}

			pkt := seccompDataPacket(int32(nr), uint32(native.ID))
			if got := runSeccompVM(t, pkt); got != uint32(seccomp.ActionAllow) {
				t.Errorf("action for %s (nr=%d) = %#x, want ActionAllow (%#x)", name, nr, got, uint32(seccomp.ActionAllow))
			}
		})
	}
}

// seccompDataPacketWithArg0 extends seccompDataPacket with arg0, the
// syscall's own first argument - clone's own flags, for this
// package's tests - written as two big-endian 32-bit halves at the
// exact byte offsets go-seccomp-bpf's own LdLo/LdHi read on a
// little-endian machine (argumentOffset=16, low half first then high;
// see assembler.go), which is what every architecture this helper
// ships for is. Like seccompDataPacket itself, this layout is only
// for driving the VM in a test, never written anywhere a real kernel
// reads from.
func seccompDataPacketWithArg0(nr int32, archID uint32, arg0 uint64) []byte {
	buf := seccompDataPacket(nr, archID)
	binary.BigEndian.PutUint32(buf[16:20], uint32(arg0))
	binary.BigEndian.PutUint32(buf[20:24], uint32(arg0>>32))
	return buf
}

func TestSeccompInstructions_RefusesMemfdSyscallsOnTheNativeArch(t *testing.T) {
	t.Parallel()

	native, err := arch.GetInfo("")
	if err != nil {
		t.Fatalf("arch.GetInfo() error = %v", err)
	}

	want := uint32(seccomp.ActionErrno) | errnoEPERM

	for _, name := range memfdSyscalls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			nr, ok := native.SyscallNames[name]
			if !ok {
				t.Fatalf("native arch %s has no syscall number for %q", native.Name, name)
			}

			pkt := seccompDataPacket(int32(nr), uint32(native.ID))
			if got := runSeccompVM(t, pkt); got != want {
				t.Errorf("action for %s (nr=%d) = %#x, want ActionErrno|EPERM (%#x)", name, nr, got, want)
			}
		})
	}
}

func TestSeccompInstructions_RefusesForkAndVforkOnTheNativeArch(t *testing.T) {
	t.Parallel()

	native, err := arch.GetInfo("")
	if err != nil {
		t.Fatalf("arch.GetInfo() error = %v", err)
	}

	want := uint32(seccomp.ActionErrno) | errnoEPERM

	for _, name := range processCreationSyscalls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			nr, ok := native.SyscallNames[name]
			if !ok {
				t.Fatalf("native arch %s has no syscall number for %q", native.Name, name)
			}

			pkt := seccompDataPacket(int32(nr), uint32(native.ID))
			if got := runSeccompVM(t, pkt); got != want {
				t.Errorf("action for %s (nr=%d) = %#x, want ActionErrno|EPERM (%#x)", name, nr, got, want)
			}
		})
	}
}

func TestSeccompInstructions_CloneWithoutThreadFlagIsRefused(t *testing.T) {
	t.Parallel()

	native, err := arch.GetInfo("")
	if err != nil {
		t.Fatalf("arch.GetInfo() error = %v", err)
	}
	nr, ok := native.SyscallNames["clone"]
	if !ok {
		t.Fatalf("native arch %s has no syscall number for clone", native.Name)
	}

	// Any flags value at all, as long as cloneThreadFlag's own bit is
	// clear: fork() and a one-off clone neither ever sets it, and this
	// filter must refuse both the same way.
	pkt := seccompDataPacketWithArg0(int32(nr), uint32(native.ID), 0)

	want := uint32(seccomp.ActionErrno) | errnoEPERM
	if got := runSeccompVM(t, pkt); got != want {
		t.Errorf("action for clone(flags=0) = %#x, want ActionErrno|EPERM (%#x)", got, want)
	}
}

func TestSeccompInstructions_CloneWithThreadFlagIsAllowed(t *testing.T) {
	t.Parallel()

	native, err := arch.GetInfo("")
	if err != nil {
		t.Fatalf("arch.GetInfo() error = %v", err)
	}
	nr, ok := native.SyscallNames["clone"]
	if !ok {
		t.Fatalf("native arch %s has no syscall number for clone", native.Name)
	}

	// glibc's own pthread_create also sets further flags alongside
	// cloneThreadFlag (CLONE_VM, CLONE_SIGHAND and the rest clone(2)
	// requires to go with it); this only needs the one bit the filter
	// itself actually checks.
	pkt := seccompDataPacketWithArg0(int32(nr), uint32(native.ID), cloneThreadFlag)

	if got := runSeccompVM(t, pkt); got != uint32(seccomp.ActionAllow) {
		t.Errorf("action for clone(flags=CLONE_THREAD) = %#x, want ActionAllow (%#x)", got, uint32(seccomp.ActionAllow))
	}
}

func TestSeccompInstructions_Clone3IsRefusedWithENOSYS(t *testing.T) {
	t.Parallel()

	native, err := arch.GetInfo("")
	if err != nil {
		t.Fatalf("arch.GetInfo() error = %v", err)
	}
	nr, ok := native.SyscallNames["clone3"]
	if !ok {
		t.Fatalf("native arch %s has no syscall number for clone3", native.Name)
	}

	pkt := seccompDataPacket(int32(nr), uint32(native.ID))

	want := uint32(seccomp.ActionErrno) | errnoENOSYS
	if got := runSeccompVM(t, pkt); got != want {
		t.Errorf("action for clone3 = %#x, want ActionErrno|ENOSYS (%#x)", got, want)
	}
}

func TestSeccompInstructions_X32SyscallNumberIsRefusedWithENOSYS(t *testing.T) {
	t.Parallel()

	// The library's own x32Filter (filter.go) exists for exactly this:
	// the x32 ABI shares x86_64's own AUDIT_ARCH_X86_64 arch value but
	// signals itself through the syscall number's own top bit
	// (__X32_SYSCALL_BIT, 0x40000000), which would otherwise let a
	// crafted x32 syscall number dodge the plain numeric comparisons
	// the denylist does. This package's own arch gate runs first but
	// does not touch syscall numbers at all, so this must still work
	// exactly as the library built it.
	const x32SyscallBit = 0x40000000

	nr, ok := arch.X86_64.SyscallNames["read"]
	if !ok {
		t.Fatal("x86_64 has no syscall number for read")
	}

	pkt := seccompDataPacket(int32(nr|x32SyscallBit), uint32(arch.X86_64.ID))

	want := uint32(seccomp.ActionErrno) | errnoENOSYS
	if got := runSeccompVM(t, pkt); got != want {
		t.Errorf("action = %#x, want ActionErrno|ENOSYS (%#x)", got, want)
	}
}
