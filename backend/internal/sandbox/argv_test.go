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
		Binary:    "/usr/bin/ffmpeg",
		Args:      []string{"-i", InputPath, OutputDirPath + "/out.mp4"},
		Input:     "/host/in.mp4",
		OutputDir: "/host/out",
	}

	name, args := buildArgv(spec, "/usr/bin/bwrap", "/usr/bin/prlimit", "/usr/bin/env")

	if name != "/usr/bin/env" {
		t.Fatalf("name = %q, want /usr/bin/env", name)
	}

	want := []string{
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
		"--tmpfs", "/tmp",
		"--ro-bind", "/host/in.mp4", "/work/in",
		"--bind", "/host/out", "/work/out",
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

	args := bwrapArgs(Spec{Binary: "/usr/bin/true", OutputDir: "/host/out"})

	// --ro-bind itself still appears once, for the base /usr mount
	// every job gets; what must be absent is InputPath, which only a
	// bound Input would ever introduce.
	if slices.Contains(args, InputPath) {
		t.Fatalf("args contains %q with no Input set: %q", InputPath, args)
	}
	if !slices.Contains(args, "--bind") {
		t.Fatalf("args missing --bind for OutputDir: %q", args)
	}
}

func TestBwrapArgs_BindsInputWhenGiven(t *testing.T) {
	t.Parallel()

	args := bwrapArgs(Spec{Binary: "/usr/bin/true", Input: "/host/in", OutputDir: "/host/out"})

	// Find InputPath itself, then check the two arguments right
	// before it: the base /usr mount also uses --ro-bind, so this
	// cannot just look for that flag's first occurrence.
	want := []string{"--ro-bind", "/host/in", InputPath}
	i := slices.Index(args, InputPath)
	if i < 2 || !slices.Equal(args[i-2:i+1], want) {
		t.Fatalf("args = %q, want to contain %q", args, want)
	}
}

func TestSpecValidate(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		spec    Spec
		wantErr bool
	}{
		"valid": {
			spec: Spec{Binary: "/usr/bin/ffmpeg", OutputDir: "/host/out"},
		},
		"missing binary": {
			spec:    Spec{OutputDir: "/host/out"},
			wantErr: true,
		},
		"relative binary": {
			spec:    Spec{Binary: "ffmpeg", OutputDir: "/host/out"},
			wantErr: true,
		},
		"missing output dir": {
			spec:    Spec{Binary: "/usr/bin/ffmpeg"},
			wantErr: true,
		},
		"input named InputBaseName is fine": {
			spec: Spec{Binary: "/usr/bin/ffmpeg", OutputDir: "/host/out", Input: "/host/job/" + InputBaseName},
		},
		"input not named InputBaseName": {
			// A caller's own file name reaching this field at all is
			// exactly the mistake validate exists to catch: bwrap's own
			// argv would otherwise carry it (see Spec's own doc
			// comment).
			spec:    Spec{Binary: "/usr/bin/ffmpeg", OutputDir: "/host/out", Input: "/host/attachment-original-name.mp4"},
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
