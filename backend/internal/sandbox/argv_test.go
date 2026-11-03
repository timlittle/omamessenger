package sandbox

// argv_test.go tests buildArgv, bwrapArgs and Spec.validate directly,
// as an internal test: they are pure and unexported, Run never
// exposes the argv it actually builds, and the fake command seam used
// to test Run's own process lifecycle (see process_internal_test.go)
// only needs some argv that runs a controllable trivial program, not
// one that exhaustively covers every flag this file checks.

import (
	"slices"
	"testing"
)

func TestBuildArgv_FullSpec(t *testing.T) {
	t.Parallel()

	spec := Spec{
		Binary: "/usr/bin/ffmpeg",
		Args:   []string{"-i", InputPath, OutputDirPath + "/out.mp4"},
		Input:  "/host/in.mp4",
		Output: "/host/out/out.mp4",
	}

	name, args := buildArgv(spec, toolPaths{
		nice: "/usr/bin/nice", ionice: "/usr/bin/ionice", env: "/usr/bin/env", prlimit: "/usr/bin/prlimit", bwrap: "/usr/bin/bwrap",
	})

	if name != "/usr/bin/nice" {
		t.Fatalf("name = %q, want /usr/bin/nice", name)
	}

	want := []string{
		"-n", "19",
		"--",
		"/usr/bin/ionice",
		"-c", "3",
		"--",
		"/usr/bin/env",
		"--ignore-signal=XFSZ,XCPU",
		"--",
		"/usr/bin/prlimit",
		"--core=0", "--cpu=900", "--as=4294967296", "--fsize=536870912", "--nofile=64",
		"--",
		"/usr/bin/bwrap",
		"--ro-bind", "/usr", "/usr",
		"--symlink", "usr/lib", "/lib",
		"--symlink", "usr/lib", "/lib64",
		"--symlink", "usr/bin", "/bin",
		"--proc", "/proc",
		"--dev", "/dev",
		"--size", "16777216", "--tmpfs", "/dev/shm",
		"--remount-ro", "/dev",
		"--size", "16777216", "--tmpfs", "/tmp",
		"--ro-bind", "/host/in.mp4", "/work/in",
		"--tmpfs", "/work/out",
		"--bind", "/host/out/out.mp4", "/work/out/out.mp4",
		"--remount-ro", "/work/out",
		"--unshare-all",
		"--unshare-user",
		"--disable-userns",
		"--die-with-parent",
		"--new-session",
		"--clearenv",
		"--setenv", "PATH", "/usr/bin:/bin",
		"--setenv", "LANG", "C.UTF-8",
		"--chdir", "/work",
		"--seccomp", "3",
		"--remount-ro", "/",
		"--",
		"/usr/bin/ffmpeg",
		"-i", "/work/in", "/work/out/out.mp4",
	}

	if !slices.Equal(args, want) {
		t.Fatalf("args =\n%q\nwant\n%q", args, want)
	}
}

func TestBwrapArgs_OmitsInputBindWhenNoneGiven(t *testing.T) {
	t.Parallel()

	args := bwrapArgs(Spec{Binary: "/usr/bin/true", Output: "/host/out/out.bin"})

	// --ro-bind itself still appears once, for the base /usr mount
	// every job gets; what must be absent is InputPath, which only a
	// bound Input would ever introduce.
	if slices.Contains(args, InputPath) {
		t.Fatalf("args contains %q with no Input set: %q", InputPath, args)
	}
	if !slices.Contains(args, "--bind") {
		t.Fatalf("args missing --bind for Output: %q", args)
	}
}

func TestBwrapArgs_BindsInputWhenGiven(t *testing.T) {
	t.Parallel()

	args := bwrapArgs(Spec{Binary: "/usr/bin/true", Input: "/host/in", Output: "/host/out/out.bin"})

	// Find InputPath itself, then check the two arguments right
	// before it: the base /usr mount also uses --ro-bind, so this
	// cannot just look for that flag's first occurrence.
	want := []string{"--ro-bind", "/host/in", InputPath}
	i := slices.Index(args, InputPath)
	if i < 2 || !slices.Equal(args[i-2:i+1], want) {
		t.Fatalf("args = %q, want to contain %q", args, want)
	}
}

func TestBwrapArgs_BindsOutputAsASingleFileUnderAReadOnlyParent(t *testing.T) {
	t.Parallel()

	args := bwrapArgs(Spec{Binary: "/usr/bin/true", Output: "/host/job/out.mp4"})

	// The three flags that make OutputDirPath hold nothing but the one
	// bound file (see bwrapArgs' own doc comment): a fresh tmpfs, the
	// file itself bound in at its own base name, then that tmpfs
	// remounted read-only - in that exact order, since binding the
	// file before the tmpfs exists, or remounting read-only before the
	// bind, would not do what the comment says it does.
	want := []string{
		"--tmpfs", OutputDirPath,
		"--bind", "/host/job/out.mp4", OutputDirPath + "/out.mp4",
		"--remount-ro", OutputDirPath,
	}

	// OutputDirPath itself appears nowhere else in this spec's own
	// args (the bind target is OutputDirPath plus a suffix, a
	// different string), so its first occurrence is exactly where
	// "--tmpfs" introduces it.
	i := slices.Index(args, OutputDirPath)
	if i < 1 || args[i-1] != "--tmpfs" || i-1+len(want) > len(args) || !slices.Equal(args[i-1:i-1+len(want)], want) {
		t.Fatalf("args = %q, want to contain %q in order", args, want)
	}
}

func TestBwrapArgs_CapsAndIsolatesDevShm(t *testing.T) {
	t.Parallel()

	args := bwrapArgs(Spec{Binary: "/usr/bin/true", Output: "/host/out/out.bin"})

	// --dev /dev itself creates /dev/shm with no size of its own
	// (confirmed directly through a real bwrap: it shares /dev's own
	// uncapped tmpfs); mounting a second, explicitly sized tmpfs at
	// /dev/shm right after replaces that, and /dev is then remounted
	// read-only - which, confirmed directly, does not affect the
	// separate tmpfs now mounted at /dev/shm, the same way remounting
	// OutputDirPath read-only below does not affect the file bound
	// inside it.
	want := []string{
		"--dev", "/dev",
		"--size", "16777216", "--tmpfs", "/dev/shm",
		"--remount-ro", "/dev",
	}

	i := slices.Index(args, "--dev")
	if i < 0 || i+len(want) > len(args) || !slices.Equal(args[i:i+len(want)], want) {
		t.Fatalf("args = %q, want to contain %q in order starting at --dev", args, want)
	}
}

func TestBwrapArgs_RemountsRootReadOnlyAsTheFinalFlag(t *testing.T) {
	t.Parallel()

	args := bwrapArgs(Spec{Binary: "/usr/bin/true", Output: "/host/out/out.bin"})

	// Last, not first: every other mount - /tmp, the input bind, the
	// output file's own three-flag bind, /dev/shm - must already be
	// in place before the root tmpfs bwrap creates by default is
	// remounted read-only, confirmed directly through a real bwrap
	// that this exact position leaves every one of those still
	// writable (or readable) as its own doc comment says, while a new
	// file anywhere else under / fails.
	want := []string{"--remount-ro", "/"}
	if len(args) < 2 || !slices.Equal(args[len(args)-2:], want) {
		t.Fatalf("args = %q, want to end with %q", args, want)
	}
}

func TestSpecValidate(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		spec    Spec
		wantErr bool
	}{
		"valid": {
			spec: Spec{Binary: "/usr/bin/ffmpeg", Output: "/host/out/out.mp4"},
		},
		"missing binary": {
			spec:    Spec{Output: "/host/out/out.mp4"},
			wantErr: true,
		},
		"relative binary": {
			spec:    Spec{Binary: "ffmpeg", Output: "/host/out/out.mp4"},
			wantErr: true,
		},
		"missing output": {
			spec:    Spec{Binary: "/usr/bin/ffmpeg"},
			wantErr: true,
		},
		"input named InputBaseName is fine": {
			spec: Spec{Binary: "/usr/bin/ffmpeg", Output: "/host/out/out.mp4", Input: "/host/job/" + InputBaseName},
		},
		"input not named InputBaseName": {
			// A caller's own file name reaching this field at all is
			// exactly the mistake validate exists to catch: bwrap's own
			// argv would otherwise carry it (see Spec's own doc
			// comment).
			spec:    Spec{Binary: "/usr/bin/ffmpeg", Output: "/host/out/out.mp4", Input: "/host/attachment-original-name.mp4"},
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := tt.spec.validate()
			if tt.wantErr && err == nil {
				t.Fatal("validate() = nil, want an error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validate() = %v, want nil", err)
			}
		})
	}
}
