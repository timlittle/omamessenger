package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func lookup(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestResolveConfig(t *testing.T) {
	tests := []struct {
		name                               string
		args                               []string
		env                                map[string]string
		wantDB                             string
		wantDemo, wantChatter, wantVersion bool
		wantErr                            bool
	}{
		{name: "default", env: map[string]string{"HOME": "/home/test"}, wantDB: "/home/test/.local/share/omamessenger/messages.db", wantChatter: true},
		{name: "xdg demo", args: []string{"--demo", "--seed", "7"}, env: map[string]string{"XDG_DATA_HOME": "/data"}, wantDB: "/data/omamessenger/demo.db", wantDemo: true, wantChatter: true},
		{name: "overrides", args: []string{"--demo", "--no-chatter", "--data-dir", "/tmp/oma", "--db", "/tmp/custom.db"}, env: map[string]string{}, wantDB: "/tmp/custom.db", wantDemo: true},
		{name: "version", args: []string{"--version"}, env: map[string]string{"HOME": "/home/test"}, wantDB: "/home/test/.local/share/omamessenger/messages.db", wantChatter: true, wantVersion: true},
		{name: "missing home", wantErr: true},
		{name: "unknown flag", args: []string{"--unknown"}, env: map[string]string{"HOME": "/home/test"}, wantErr: true},
		{name: "extra arg", args: []string{"extra"}, env: map[string]string{"HOME": "/home/test"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := resolveConfig(tt.args, lookup(tt.env))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.DBPath != tt.wantDB || cfg.Demo != tt.wantDemo || cfg.Chatter != tt.wantChatter || cfg.Version != tt.wantVersion {
				t.Fatalf("config=%+v", cfg)
			}
			if cfg.Seed == 0 {
				t.Fatal("seed was not populated")
			}
		})
	}
	if _, err := resolveConfig(nil, nil); err == nil {
		t.Fatal("nil environment lookup should fail")
	}
}

type frame struct {
	ID     int             `json:"id"`
	Event  string          `json:"event"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

func TestBuiltHelperDemoProtocolAndCleanEOF(t *testing.T) {
	temp := t.TempDir()
	binary := filepath.Join(temp, "oma-messenger-service")
	build := exec.Command("go", "build", "-mod=vendor", "-o", binary, ".")
	var buildErr bytes.Buffer
	build.Stderr = &buildErr
	if err := build.Run(); err != nil {
		t.Fatalf("build helper: %v\n%s", err, buildErr.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--demo", "--no-chatter", "--seed", "1", "--data-dir", filepath.Join(temp, "data"))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	frames := make(chan frame, 2048)
	readErr := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			var value frame
			if err := json.Unmarshal(scanner.Bytes(), &value); err != nil {
				readErr <- err
				return
			}
			frames <- value
		}
		readErr <- scanner.Err()
	}()
	send := func(id int, method string) {
		t.Helper()
		if _, err := fmt.Fprintf(stdin, `{"id":%d,"method":%q,"params":{}}`+"\n", id, method); err != nil {
			t.Fatal(err)
		}
	}
	await := func(id int, timeout time.Duration) frame {
		t.Helper()
		deadline := time.NewTimer(timeout)
		defer deadline.Stop()
		for {
			select {
			case value := <-frames:
				if value.ID == id {
					if value.Error != nil {
						t.Fatalf("RPC %d error: %s", id, value.Error)
					}
					return value
				}
			case err := <-readErr:
				t.Fatalf("helper output ended: %v\n%s", err, stderr.String())
			case <-deadline.C:
				t.Fatalf("timed out waiting for response %d", id)
			}
		}
	}
	send(1, "hello")
	hello := await(1, 5*time.Second)
	var helloResult struct {
		Protocol int  `json:"protocol"`
		Demo     bool `json:"demo"`
	}
	if err := json.Unmarshal(hello.Result, &helloResult); err != nil || helloResult.Protocol != 1 || !helloResult.Demo {
		t.Fatalf("hello=%s err=%v", hello.Result, err)
	}

	var chats []json.RawMessage
	for id := 2; id < 100 && len(chats) != 11; id++ {
		send(id, "conversations.list")
		response := await(id, 5*time.Second)
		if err := json.Unmarshal(response.Result, &chats); err != nil {
			t.Fatal(err)
		}
		if len(chats) != 11 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if len(chats) != 11 {
		t.Fatalf("expected 11 demo conversations, got %d", len(chats))
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("helper exit: %v\n%s", err, stderr.String())
		}
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("helper did not exit within 2s of stdin EOF")
	}
	if strings.Contains(stderr.String(), "messages.db") {
		t.Errorf("startup log exposed a database path: %s", stderr.String())
	}
}

func TestVersionPrintsWithoutOpeningData(t *testing.T) {
	var out, stderr bytes.Buffer
	err := run(context.Background(), strings.NewReader(""), &out, &stderr, []string{"--version"}, lookup(map[string]string{"HOME": "/definitely/not/created"}))
	if err != nil || strings.TrimSpace(out.String()) != helperVersion || stderr.Len() != 0 {
		t.Fatalf("version: out=%q stderr=%q err=%v", out.String(), stderr.String(), err)
	}
	if _, err := os.Stat("/definitely/not/created"); !os.IsNotExist(err) {
		t.Fatalf("version unexpectedly used data dir: %v", err)
	}
}
