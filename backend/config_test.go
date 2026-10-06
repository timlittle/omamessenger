package main

import "testing"

func TestResolveConfig(t *testing.T) {
	t.Parallel()

	home := map[string]string{"HOME": "/home/test"}
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		want    config
		wantErr bool
	}{
		{
			name: "defaults under HOME", env: home,
			want: config{dataDir: "/home/test/.local/share/omamessenger", dbPath: "/home/test/.local/share/omamessenger/messages.db"},
		},
		{
			name: "XDG_DATA_HOME wins", env: map[string]string{"HOME": "/home/test", "XDG_DATA_HOME": "/data"},
			want: config{dataDir: "/data/omamessenger", dbPath: "/data/omamessenger/messages.db"},
		},
		{
			name: "demo has its own database", args: []string{"--demo", "--seed", "7"}, env: map[string]string{"XDG_DATA_HOME": "/data"},
			want: config{demo: true, seed: 7, dataDir: "/data/omamessenger", dbPath: "/data/omamessenger/demo.db"},
		},
		{
			name: "data-dir is used as given", args: []string{"--data-dir", "/tmp/oma", "--seed", "1"},
			want: config{seed: 1, dataDir: "/tmp/oma", dbPath: "/tmp/oma/messages.db"},
		},
		{
			name: "db overrides data-dir", args: []string{"--chatter", "--data-dir", "/tmp/oma", "--db", "/tmp/x.db", "--seed", "1"},
			want: config{seed: 1, dataDir: "/tmp/oma", dbPath: "/tmp/x.db", chatter: true},
		},
		{name: "no HOME", args: []string{"--seed", "1"}, wantErr: true},
		{name: "unknown flag", args: []string{"--unknown"}, env: home, wantErr: true},
		{name: "invalid seed", args: []string{"--seed", "x"}, env: home, wantErr: true},
		{name: "extra argument", args: []string{"extra"}, env: home, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveConfig(tt.args, lookup(tt.env))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveConfig = %+v, want an error", got)
				}

				return
			}

			if tt.want.seed == 0 {
				tt.want.seed = got.seed // the default seed is the clock
			}

			if err != nil || got != tt.want {
				t.Errorf("resolveConfig = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

// lookup returns an environment lookup over values.
func lookup(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}
