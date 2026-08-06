package pkgbuild

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cliTestPipeline stubs every runner; envCalls records each RunEnv invocation.
type cliEnvCall struct {
	dir  string
	env  []string
	args []string
}

func cliTestPipeline(calls *[]string, envCalls *[]cliEnvCall, envErr func(target string) error) Pipeline {
	record := func(name string, args []string) {
		*calls = append(*calls, name+" "+strings.Join(args, " "))
	}
	return Pipeline{
		Run: func(dir, name string, args ...string) (string, error) {
			record(name, args)
			return "", nil
		},
		RunEnv: func(dir string, extraEnv []string, name string, args ...string) (string, error) {
			record("env:"+name, args)
			*envCalls = append(*envCalls, cliEnvCall{dir: dir, env: extraEnv, args: args})
			if envErr != nil {
				for _, kv := range extraEnv {
					if osName, ok := strings.CutPrefix(kv, "GOOS="); ok {
						if err := envErr(osName); err != nil {
							return "compile output", err
						}
					}
				}
			}
			return "", nil
		},
		PnpmStream: func(_ func(string), _, name string, args ...string) (string, error) {
			record(name, args)
			return "", nil
		},
		ExpoStream: func(_ func(string), _, name string, args ...string) (string, error) {
			record(name, args)
			return "", nil
		},
		Stage: func(appDir string) (string, error) {
			return filepath.Join(appDir, "release-staging", "rel-1"), nil
		},
		NativeExport: func(ProgressSink, string, string, string) ([]BundleMeta, error) {
			return nil, nil
		},
	}
}

func mkCliDir(t *testing.T) string {
	t.Helper()
	build := t.TempDir()
	if err := os.MkdirAll(filepath.Join(build, "tinycld", "cli"), 0o755); err != nil {
		t.Fatal(err)
	}
	return build
}

func TestPipelineExecute_CLICrossCompile(t *testing.T) {
	build := mkCliDir(t)
	var calls []string
	var envCalls []cliEnvCall
	p := cliTestPipeline(&calls, &envCalls, nil)

	if _, err := p.Execute(NopSink(), build, "build-1"); err != nil {
		t.Fatal(err)
	}

	if len(envCalls) != len(CLITargets) {
		t.Fatalf("cross-compile calls = %d, want %d", len(envCalls), len(CLITargets))
	}
	cliDir := filepath.Join(build, "tinycld", "cli")
	distDir := filepath.Join(build, "tinycld", CLIDistDirName)
	for i, c := range envCalls {
		target := CLITargets[i]
		if c.dir != cliDir {
			t.Fatalf("call %d dir = %q, want %q", i, c.dir, cliDir)
		}
		wantEnv := []string{"GOOS=" + target.OS, "GOARCH=" + target.Arch, "CGO_ENABLED=0"}
		if fmt.Sprint(c.env) != fmt.Sprint(wantEnv) {
			t.Fatalf("call %d env = %v, want %v", i, c.env, wantEnv)
		}
		wantOut := filepath.Join(distDir, target.FileName())
		joined := strings.Join(c.args, " ")
		if !strings.Contains(joined, "-o "+wantOut) {
			t.Fatalf("call %d args %q missing -o %s", i, joined, wantOut)
		}
		if !strings.HasPrefix(joined, "build -trimpath") {
			t.Fatalf("call %d args = %q", i, joined)
		}
	}
	if _, err := os.Stat(distDir); err != nil {
		t.Fatalf("cli-dist not created: %v", err)
	}

	// ordered after the server go build, before expo export
	joined := strings.Join(calls, " | ")
	server := strings.Index(joined, "go build -o")
	cross := strings.Index(joined, "env:go build -trimpath")
	expo := strings.Index(joined, "expo")
	if !(server >= 0 && cross > server && expo > cross) {
		t.Fatalf("bad step order: %s", joined)
	}
}

func TestPipelineExecute_CLIFailureDoesNotFailBuild(t *testing.T) {
	build := mkCliDir(t)
	var calls []string
	var envCalls []cliEnvCall
	p := cliTestPipeline(&calls, &envCalls, func(string) error {
		return errors.New("compile boom")
	})

	if _, err := p.Execute(NopSink(), build, "build-1"); err != nil {
		t.Fatalf("a CLI compile failure must never fail the install: %v", err)
	}
	if len(envCalls) != len(CLITargets) {
		t.Fatalf("one failure must not stop the other targets: %d calls", len(envCalls))
	}
	if !strings.Contains(strings.Join(calls, " | "), "expo") {
		t.Fatal("later steps must still run after CLI failures")
	}
}

func TestPipelineExecute_CLISingleTargetFailureContinues(t *testing.T) {
	build := mkCliDir(t)
	var calls []string
	var envCalls []cliEnvCall
	p := cliTestPipeline(&calls, &envCalls, func(target string) error {
		if target == "windows" {
			return errors.New("windows-only boom")
		}
		return nil
	})

	if _, err := p.Execute(NopSink(), build, "build-1"); err != nil {
		t.Fatal(err)
	}
	if len(envCalls) != len(CLITargets) {
		t.Fatalf("all targets must be attempted: %d calls", len(envCalls))
	}
}

func TestPipelineExecute_CLISkippedWithoutCliDir(t *testing.T) {
	build := t.TempDir() // no tinycld/cli dir
	var calls []string
	var envCalls []cliEnvCall
	p := cliTestPipeline(&calls, &envCalls, nil)

	if _, err := p.Execute(NopSink(), build, "build-1"); err != nil {
		t.Fatal(err)
	}
	if len(envCalls) != 0 {
		t.Fatalf("RunEnv must not be called without a cli/ module: %d calls", len(envCalls))
	}
	if _, err := os.Stat(filepath.Join(build, "tinycld", CLIDistDirName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cli-dist must not be created when the build has no cli module")
	}
}

func TestCLITargetNames(t *testing.T) {
	cases := []struct {
		target   CLITarget
		platform string
		file     string
		download string
	}{
		{CLITarget{"darwin", "arm64"}, "darwin-arm64", "tinycld-darwin-arm64", "tinycld"},
		{CLITarget{"linux", "amd64"}, "linux-amd64", "tinycld-linux-amd64", "tinycld"},
		{CLITarget{"windows", "amd64"}, "windows-amd64", "tinycld-windows-amd64.exe", "tinycld.exe"},
	}
	for _, c := range cases {
		if got := c.target.Platform(); got != c.platform {
			t.Errorf("Platform() = %q, want %q", got, c.platform)
		}
		if got := c.target.FileName(); got != c.file {
			t.Errorf("FileName() = %q, want %q", got, c.file)
		}
		if got := c.target.DownloadName(); got != c.download {
			t.Errorf("DownloadName() = %q, want %q", got, c.download)
		}
	}
}
