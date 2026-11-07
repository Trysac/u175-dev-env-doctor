package devenvdoctor_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Trysac/u175-dev-env-doctor"
)

// fakeRunner implements devenvdoctor.Runner for unit testing.
type fakeRunner struct {
	handler func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func (f fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if f.handler != nil {
		return f.handler(ctx, name, args...)
	}
	return nil, devenvdoctor.ErrNotFound
}

// Acceptance Item 1:
// CompareVersions("1.22", "1.22.0") == 0, CompareVersions("1.9", "1.10") == -1 and
// CompareVersions("2", "1.99.99") == 1, and CompareVersions("1.a", "1") returns an error
// matching ErrInvalidVersion with errors.Is
func TestCompareVersions_Acceptance(t *testing.T) {
	cmp, err := devenvdoctor.CompareVersions("1.22", "1.22.0")
	if err != nil || cmp != 0 {
		t.Fatalf("CompareVersions(\"1.22\", \"1.22.0\") = %d, %v; want 0, nil", cmp, err)
	}

	cmp, err = devenvdoctor.CompareVersions("1.9", "1.10")
	if err != nil || cmp != -1 {
		t.Fatalf("CompareVersions(\"1.9\", \"1.10\") = %d, %v; want -1, nil", cmp, err)
	}

	cmp, err = devenvdoctor.CompareVersions("2", "1.99.99")
	if err != nil || cmp != 1 {
		t.Fatalf("CompareVersions(\"2\", \"1.99.99\") = %d, %v; want 1, nil", cmp, err)
	}

	_, err = devenvdoctor.CompareVersions("1.a", "1")
	if !errors.Is(err, devenvdoctor.ErrInvalidVersion) {
		t.Fatalf("CompareVersions(\"1.a\", \"1\") error = %v; want ErrInvalidVersion", err)
	}
}

func TestCompareVersions_EdgeCases(t *testing.T) {
	invalids := []string{
		"",
		"1.2.3.4",
		"-1.2",
		"+1.2",
		"1234567890", // 10 digits
		"1..2",
		".1",
		"1.",
	}
	for _, s := range invalids {
		if _, err := devenvdoctor.CompareVersions(s, "1.0"); !errors.Is(err, devenvdoctor.ErrInvalidVersion) {
			t.Errorf("expected ErrInvalidVersion for %q on left, got %v", s, err)
		}
		if _, err := devenvdoctor.CompareVersions("1.0", s); !errors.Is(err, devenvdoctor.ErrInvalidVersion) {
			t.Errorf("expected ErrInvalidVersion for %q on right, got %v", s, err)
		}
	}
}

// Acceptance Item 2:
// With a fake runner returning "go version go1.23.4 windows/amd64" for go, pattern
// "go(\\d+\\.\\d+(?:\\.\\d+)?)" and min "1.22", the result has Status "ok", Found "1.23.4"
// and Want ">=1.22"; output "go version go1.21.0 linux/amd64" gives Status "outdated"
func TestCheck_GoVersion_Acceptance(t *testing.T) {
	pattern := `go(\d+\.\d+(?:\.\d+)?)`
	cfgJSON := fmt.Sprintf(`{
		"tools": [
			{"name": "go", "command": ["go", "version"], "pattern": %q, "min": "1.22"}
		]
	}`, pattern)

	cfg, err := devenvdoctor.LoadConfig(strings.NewReader(cfgJSON))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Case 1: 1.23.4 ok
	runner1 := fakeRunner{
		handler: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return []byte("go version go1.23.4 windows/amd64"), nil
		},
	}
	rep1 := devenvdoctor.Check(context.Background(), cfg, devenvdoctor.Deps{Runner: runner1})
	if len(rep1.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(rep1.Results))
	}
	r1 := rep1.Results[0]
	if r1.Status != "ok" || r1.Found != "1.23.4" || r1.Want != ">=1.22" {
		t.Fatalf("result 1 = %+v; want Status ok, Found 1.23.4, Want >=1.22", r1)
	}
	if !rep1.OK {
		t.Fatalf("expected Report.OK true, got false")
	}

	// Case 2: 1.21.0 outdated
	runner2 := fakeRunner{
		handler: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return []byte("go version go1.21.0 linux/amd64"), nil
		},
	}
	rep2 := devenvdoctor.Check(context.Background(), cfg, devenvdoctor.Deps{Runner: runner2})
	if len(rep2.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(rep2.Results))
	}
	r2 := rep2.Results[0]
	if r2.Status != "outdated" {
		t.Fatalf("result 2 Status = %q; want outdated", r2.Status)
	}
	if rep2.OK {
		t.Fatalf("expected Report.OK false for outdated tool")
	}
}

// Acceptance Item 3:
// node output "v25.1.0\n" with min "20.0.0" and max "25" gives Status "unsupported" and
// Want ">=20.0.0 <25"; a runner error matching ErrNotFound gives "missing"; output "nodejs"
// gives "unknown_version"
func TestCheck_NodeVersion_Acceptance(t *testing.T) {
	cfgJSON := `{
		"tools": [
			{"name": "node", "command": ["node", "--version"], "pattern": "v(\\d+\\.\\d+(?:\\.\\d+)?)", "min": "20.0.0", "max": "25"}
		]
	}`
	cfg, err := devenvdoctor.LoadConfig(strings.NewReader(cfgJSON))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Case 1: unsupported
	runner1 := fakeRunner{
		handler: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return []byte("v25.1.0\n"), nil
		},
	}
	rep1 := devenvdoctor.Check(context.Background(), cfg, devenvdoctor.Deps{Runner: runner1})
	if rep1.Results[0].Status != "unsupported" || rep1.Results[0].Want != ">=20.0.0 <25" {
		t.Fatalf("rep1 = %+v; want Status unsupported and Want >=20.0.0 <25", rep1.Results[0])
	}

	// Case 2: runner error matching ErrNotFound gives "missing"
	runner2 := fakeRunner{
		handler: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("wrapper: %w", devenvdoctor.ErrNotFound)
		},
	}
	rep2 := devenvdoctor.Check(context.Background(), cfg, devenvdoctor.Deps{Runner: runner2})
	if rep2.Results[0].Status != "missing" {
		t.Fatalf("rep2 status = %q; want missing", rep2.Results[0].Status)
	}

	// Case 3: output "nodejs" gives "unknown_version"
	runner3 := fakeRunner{
		handler: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return []byte("nodejs"), nil
		},
	}
	rep3 := devenvdoctor.Check(context.Background(), cfg, devenvdoctor.Deps{Runner: runner3})
	if rep3.Results[0].Status != "unknown_version" {
		t.Fatalf("rep3 status = %q; want unknown_version", rep3.Results[0].Status)
	}
}

// Acceptance Item 4:
// A runner that blocks until its context ends, with a tool TimeoutMS of 20, gives
// Status "timeout" and Check returns in under 1 s
func TestCheck_Timeout_Acceptance(t *testing.T) {
	cfg := devenvdoctor.Config{
		Tools: []devenvdoctor.Tool{
			{
				Name:      "blocker",
				Command:   []string{"blocker"},
				Pattern:   `(\d+)`,
				TimeoutMS: 20,
			},
		},
	}
	runner := fakeRunner{
		handler: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	start := time.Now()
	rep := devenvdoctor.Check(context.Background(), cfg, devenvdoctor.Deps{Runner: runner})
	duration := time.Since(start)

	if duration >= 1*time.Second {
		t.Fatalf("Check took %v, must be under 1s", duration)
	}
	if len(rep.Results) != 1 || rep.Results[0].Status != "timeout" {
		t.Fatalf("rep.Results[0].Status = %q, want timeout", rep.Results[0].Status)
	}
	if rep.OK {
		t.Fatalf("expected Report.OK to be false on timeout")
	}
}

// Acceptance Item 5:
// A required env var that Getenv reports unset gives Status "missing_env", a missing
// file in the fs.FS gives "missing_file", a failing tool marked optional leaves Report.OK
// true while a failing required tool makes it false, and results keep the order tools,
// env, files as declared
func TestCheck_EnvAndFilesAndOptional_Acceptance(t *testing.T) {
	cfg := devenvdoctor.Config{
		Tools: []devenvdoctor.Tool{
			{
				Name:     "opt-tool",
				Command:  []string{"opt"},
				Pattern:  `(\d+)`,
				Optional: true,
			},
		},
		Env: []devenvdoctor.EnvVar{
			{Name: "REQUIRED_ENV", Required: true},
		},
		Files: []string{"missing_config.yaml"},
	}

	memFS := fstest.MapFS{}
	deps := devenvdoctor.Deps{
		Runner: fakeRunner{handler: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return nil, devenvdoctor.ErrNotFound
		}},
		Getenv: func(key string) (string, bool) {
			return "", false
		},
		FS: memFS,
	}

	rep := devenvdoctor.Check(context.Background(), cfg, deps)

	if len(rep.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(rep.Results))
	}

	// Check kinds and ordering: tools, env, files
	if rep.Results[0].Kind != "tool" || rep.Results[0].Name != "opt-tool" || rep.Results[0].Status != "missing" {
		t.Errorf("result 0 mismatch: %+v", rep.Results[0])
	}
	if rep.Results[1].Kind != "env" || rep.Results[1].Name != "REQUIRED_ENV" || rep.Results[1].Status != "missing_env" {
		t.Errorf("result 1 mismatch: %+v", rep.Results[1])
	}
	if rep.Results[2].Kind != "file" || rep.Results[2].Name != "missing_config.yaml" || rep.Results[2].Status != "missing_file" {
		t.Errorf("result 2 mismatch: %+v", rep.Results[2])
	}
	if rep.OK {
		t.Errorf("rep.OK should be false due to missing required env and missing file")
	}

	// Test that optional tool failing alone leaves Report.OK true
	cfgOptOnly := devenvdoctor.Config{
		Tools: []devenvdoctor.Tool{
			{
				Name:     "opt-tool",
				Command:  []string{"opt"},
				Pattern:  `(\d+)`,
				Optional: true,
			},
		},
	}
	repOptOnly := devenvdoctor.Check(context.Background(), cfgOptOnly, deps)
	if !repOptOnly.OK {
		t.Errorf("repOptOnly.OK must be true when only optional tool fails")
	}

	// Test that required tool failing makes Report.OK false
	cfgReqOnly := devenvdoctor.Config{
		Tools: []devenvdoctor.Tool{
			{
				Name:     "req-tool",
				Command:  []string{"req"},
				Pattern:  `(\d+)`,
				Optional: false,
			},
		},
	}
	repReqOnly := devenvdoctor.Check(context.Background(), cfgReqOnly, deps)
	if repReqOnly.OK {
		t.Errorf("repReqOnly.OK must be false when required tool fails")
	}
}

// Acceptance Item 6:
// Text output for the ok go result is exactly "ok\tgo\t1.23.4\t>=1.22\n", and a missing
// tool prints "-" in the found column
func TestFormatText_Acceptance(t *testing.T) {
	rep := devenvdoctor.Report{
		OK: true,
		Results: []devenvdoctor.Result{
			{
				Kind:   "tool",
				Name:   "go",
				Status: "ok",
				Found:  "1.23.4",
				Want:   ">=1.22",
			},
			{
				Kind:   "tool",
				Name:   "rustc",
				Status: "missing",
				Found:  "",
				Want:   ">=1.80",
			},
		},
	}

	var buf bytes.Buffer
	if err := devenvdoctor.FormatText(&buf, rep); err != nil {
		t.Fatalf("FormatText failed: %v", err)
	}

	expected := "ok\tgo\t1.23.4\t>=1.22\nmissing\trustc\t-\t>=1.80\n"
	if buf.String() != expected {
		t.Fatalf("FormatText output = %q; want %q", buf.String(), expected)
	}
}

// Acceptance Item 7:
// Run([]string{"--config", path}, stdout, stderr) returns 0 when every required check
// passes, 1 when one fails and 2 for a missing or invalid config, and --format json
// prints an object with keys "ok" and "results"
func TestRun_Acceptance(t *testing.T) {
	tmpDir := t.TempDir()
	okPath := filepath.Join(tmpDir, "ok.json")
	failPath := filepath.Join(tmpDir, "fail.json")
	badPath := filepath.Join(tmpDir, "bad.json")

	// OK config: file exists, env exists
	testFile := filepath.Join(tmpDir, "hello.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_DOC_VAR", "1")

	okCfg := fmt.Sprintf(`{
		"env": [{"name": "TEST_DOC_VAR", "required": true}],
		"files": ["hello.txt"]
	}`)
	if err := os.WriteFile(okPath, []byte(okCfg), 0o600); err != nil {
		t.Fatal(err)
	}

	// Fail config: missing required file
	failCfg := `{
		"files": ["does_not_exist.txt"]
	}`
	if err := os.WriteFile(failPath, []byte(failCfg), 0o600); err != nil {
		t.Fatal(err)
	}

	// Bad config: invalid JSON
	if err := os.WriteFile(badPath, []byte("{bad json"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer

	// Run ok
	code := devenvdoctor.Run([]string{"--config", okPath, "--root", tmpDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run ok returned %d, want 0; stderr: %s", code, stderr.String())
	}

	// Run ok with --format json
	stdout.Reset()
	stderr.Reset()
	code = devenvdoctor.Run([]string{"--config", okPath, "--format", "json", "--root", tmpDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run ok json returned %d, want 0; stderr: %s", code, stderr.String())
	}
	outStr := stdout.String()
	if !strings.Contains(outStr, `"ok": true`) || !strings.Contains(outStr, `"results": [`) {
		t.Fatalf("json output missing ok or results keys: %s", outStr)
	}

	// Run fail
	stdout.Reset()
	stderr.Reset()
	code = devenvdoctor.Run([]string{"--config", failPath, "--root", tmpDir}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run fail returned %d, want 1", code)
	}

	// Run invalid config file
	stdout.Reset()
	stderr.Reset()
	code = devenvdoctor.Run([]string{"--config", badPath, "--root", tmpDir}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("Run bad returned %d, want 2", code)
	}

	// Run missing config file
	stdout.Reset()
	stderr.Reset()
	code = devenvdoctor.Run([]string{"--config", filepath.Join(tmpDir, "nonexistent.json")}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("Run missing file returned %d, want 2", code)
	}
}

// Acceptance Item 8:
// LoadConfig rejects an unknown field "extra", the pattern "go\\d+" (no capture group),
// the command [], the min "1.x" and input over 1_048_576 bytes, each with an error
// matching ErrInvalidConfig
func TestLoadConfig_Rejections_Acceptance(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{
			name: "unknown field extra",
			json: `{"tools": [], "extra": "val"}`,
		},
		{
			name: "pattern go\\d+ with no capture group",
			json: `{"tools": [{"name": "go", "command": ["go"], "pattern": "go\\d+"}]}`,
		},
		{
			name: "command []",
			json: `{"tools": [{"name": "go", "command": [], "pattern": "go(\\d+)"}]}`,
		},
		{
			name: "min 1.x",
			json: `{"tools": [{"name": "go", "command": ["go"], "pattern": "go(\\d+)", "min": "1.x"}]}`,
		},
		{
			name: "input over 1_048_576 bytes",
			json: `{"tools": []}` + strings.Repeat(" ", 1_048_577),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := devenvdoctor.LoadConfig(strings.NewReader(tc.json))
			if !errors.Is(err, devenvdoctor.ErrInvalidConfig) {
				t.Fatalf("expected ErrInvalidConfig for %s, got: %v", tc.name, err)
			}
		})
	}
}

func TestLoadConfig_MoreValidations(t *testing.T) {
	invalids := []string{
		``, // empty
		`{"tools": [{"name": "", "command": ["a"], "pattern": "(a)"}]}`,                   // empty name
		`{"tools": [{"name": "tool", "command": ["a"], "pattern": "["}]}`,                 // invalid regex
		`{"tools": [{"name": "tool", "command": ["a"], "pattern": "(a)", "max": "bad"}]}`, // bad max
		`{"env": [{"name": ""}]}`,    // empty env name
		`{"files": ["/etc/passwd"]}`, // absolute path
		`{"files": ["../secret"]}`,   // parent path
	}

	for i, inv := range invalids {
		_, err := devenvdoctor.LoadConfig(strings.NewReader(inv))
		if !errors.Is(err, devenvdoctor.ErrInvalidConfig) {
			t.Errorf("case %d: expected ErrInvalidConfig, got %v", i, err)
		}
	}
}

func TestCheck_GeneralErrorHandling(t *testing.T) {
	cfg := devenvdoctor.Config{
		Tools: []devenvdoctor.Tool{
			{
				Name:    "errtool",
				Command: []string{"errtool"},
				Pattern: `(\d+)`,
			},
		},
	}

	// Runner returns general error
	runner := fakeRunner{
		handler: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return nil, errors.New("unknown execution failure")
		},
	}
	rep := devenvdoctor.Check(context.Background(), cfg, devenvdoctor.Deps{Runner: runner})
	if rep.Results[0].Status != "error" {
		t.Errorf("expected status error, got %q", rep.Results[0].Status)
	}

	// Nil runner
	repNil := devenvdoctor.Check(context.Background(), cfg, devenvdoctor.Deps{Runner: nil})
	if repNil.Results[0].Status != "error" {
		t.Errorf("expected status error for nil runner, got %q", repNil.Results[0].Status)
	}

	// Nil Getenv and FS
	cfgEnvFile := devenvdoctor.Config{
		Env:   []devenvdoctor.EnvVar{{Name: "FOO", Required: true}},
		Files: []string{"foo.txt"},
	}
	repNilDeps := devenvdoctor.Check(context.Background(), cfgEnvFile, devenvdoctor.Deps{})
	if repNilDeps.Results[0].Status != "missing_env" {
		t.Errorf("expected missing_env for nil Getenv, got %q", repNilDeps.Results[0].Status)
	}
	if repNilDeps.Results[1].Status != "missing_file" {
		t.Errorf("expected missing_file for nil FS, got %q", repNilDeps.Results[1].Status)
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER_PROCESS") != "1" {
		return
	}
	for _, arg := range os.Args {
		if arg == "--version" {
			fmt.Println("helper v1.2.3")
			os.Exit(0)
		}
		if arg == "--hang" {
			time.Sleep(2 * time.Second)
			os.Exit(0)
		}
	}
	os.Exit(0)
}

func TestExecRunner_HelperProcess(t *testing.T) {
	runner := devenvdoctor.NewExecRunner()

	// Run current test executable with helper arg
	execPath, err := os.Executable()
	if err != nil {
		t.Skip("cannot resolve executable")
	}

	t.Setenv("GO_TEST_HELPER_PROCESS", "1")
	out, err := runner.Run(context.Background(), execPath, "-test.run=^TestHelperProcess$", "--", "--version")
	if err != nil {
		t.Fatalf("ExecRunner.Run failed: %v", err)
	}
	if !strings.Contains(string(out), "helper v1.2.3") {
		t.Fatalf("ExecRunner.Run unexpected output: %s", string(out))
	}

	// Test not found binary
	_, err = runner.Run(context.Background(), "__non_existent_binary_xyz_1234__")
	if !errors.Is(err, devenvdoctor.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing binary, got %v", err)
	}
}
