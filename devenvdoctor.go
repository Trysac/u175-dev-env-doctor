package devenvdoctor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Sentinel errors returned by the package.
var (
	ErrNotFound       = errors.New("devenvdoctor: not found")
	ErrInvalidVersion = errors.New("devenvdoctor: invalid version")
	ErrInvalidConfig  = errors.New("devenvdoctor: invalid configuration")
)

const (
	defaultTimeoutMS    = 5000
	maxConfigBytes      = 1024 * 1024 // 1 MiB
	maxOutputBytes      = 64 * 1024   // 64 KiB
	maxToolNameBytes    = 64
	maxEnvNameBytes     = 64
	maxCommandArgs      = 16
	maxVersionLenDigits = 9
)

// Status values reported for checks.
const (
	StatusOK             = "ok"
	StatusMissing        = "missing"
	StatusOutdated       = "outdated"
	StatusUnsupported    = "unsupported"
	StatusUnknownVersion = "unknown_version"
	StatusTimeout        = "timeout"
	StatusError          = "error"
	StatusMissingEnv     = "missing_env"
	StatusMissingFile    = "missing_file"
)

// Config defines expected tools, environment variables, and files.
type Config struct {
	Tools []Tool   `json:"tools"`
	Env   []EnvVar `json:"env"`
	Files []string `json:"files"`
}

// Tool represents an executable check and its version constraints.
type Tool struct {
	Name      string   `json:"name"`
	Command   []string `json:"command"`
	Pattern   string   `json:"pattern"`
	Min       string   `json:"min"`
	Max       string   `json:"max,omitempty"`
	Optional  bool     `json:"optional"`
	TimeoutMS int      `json:"timeout_ms"`

	patternRe *regexp.Regexp
}

// EnvVar represents an environment variable requirement.
type EnvVar struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
}

// Runner executes system commands with context cancellation.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// Deps provides execution dependencies for environment checks.
type Deps struct {
	Runner Runner
	Getenv func(string) (string, bool)
	FS     fs.FS
}

// Result is a single check outcome.
type Result struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Found    string `json:"found,omitempty"`
	Want     string `json:"want,omitempty"`
	Optional bool   `json:"optional"`
}

// Report aggregates the results of all checks.
type Report struct {
	OK      bool     `json:"ok"`
	Results []Result `json:"results"`
}

// CompareVersions compares two version strings containing 1 to 3 dot-separated
// decimal parts with up to 9 digits each. Missing parts default to 0.
// Returns -1 if a < b, 0 if a == b, and 1 if a > b.
func CompareVersions(a, b string) (int, error) {
	pa, err := parseVersion(a)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidVersion, err)
	}
	pb, err := parseVersion(b)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidVersion, err)
	}

	for i := 0; i < 3; i++ {
		if pa[i] < pb[i] {
			return -1, nil
		}
		if pa[i] > pb[i] {
			return 1, nil
		}
	}
	return 0, nil
}

func parseVersion(s string) ([3]int64, error) {
	var res [3]int64
	if s == "" {
		return res, errors.New("empty version string")
	}
	parts := strings.Split(s, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return res, fmt.Errorf("version must have 1-3 parts, got %d", len(parts))
	}
	for i, p := range parts {
		if p == "" || len(p) > maxVersionLenDigits {
			return res, fmt.Errorf("invalid component length in %q", p)
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return res, fmt.Errorf("non-digit character in %q", p)
			}
		}
		val, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return res, err
		}
		res[i] = val
	}
	return res, nil
}

// LoadConfig reads and validates JSON configuration from r.
func LoadConfig(r io.Reader) (Config, error) {
	var cfg Config
	limit := io.LimitReader(r, maxConfigBytes+1)
	data, err := io.ReadAll(limit)
	if err != nil {
		return cfg, fmt.Errorf("%w: failed to read: %v", ErrInvalidConfig, err)
	}
	if len(data) > maxConfigBytes {
		return cfg, fmt.Errorf("%w: configuration exceeds 1 MiB limit", ErrInvalidConfig)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return cfg, fmt.Errorf("%w: empty configuration", ErrInvalidConfig)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}

	// Validate Tools
	for i := range cfg.Tools {
		t := &cfg.Tools[i]
		if len(t.Name) == 0 || len(t.Name) > maxToolNameBytes {
			return cfg, fmt.Errorf("%w: tool name must be 1-%d bytes", ErrInvalidConfig, maxToolNameBytes)
		}
		if len(t.Command) == 0 || len(t.Command) > maxCommandArgs {
			return cfg, fmt.Errorf("%w: tool %q command must have 1-%d arguments", ErrInvalidConfig, t.Name, maxCommandArgs)
		}
		if t.TimeoutMS == 0 {
			t.TimeoutMS = defaultTimeoutMS
		} else if t.TimeoutMS < 0 {
			t.TimeoutMS = defaultTimeoutMS
		}
		if t.Pattern == "" {
			return cfg, fmt.Errorf("%w: tool %q pattern cannot be empty", ErrInvalidConfig, t.Name)
		}
		re, err := regexp.Compile(t.Pattern)
		if err != nil {
			return cfg, fmt.Errorf("%w: tool %q pattern invalid: %v", ErrInvalidConfig, t.Name, err)
		}
		if re.NumSubexp() < 1 {
			return cfg, fmt.Errorf("%w: tool %q pattern must contain at least one capture group", ErrInvalidConfig, t.Name)
		}
		t.patternRe = re

		if t.Min != "" {
			if _, err := CompareVersions(t.Min, t.Min); err != nil {
				return cfg, fmt.Errorf("%w: tool %q min version invalid: %v", ErrInvalidConfig, t.Name, err)
			}
		}
		if t.Max != "" {
			if _, err := CompareVersions(t.Max, t.Max); err != nil {
				return cfg, fmt.Errorf("%w: tool %q max version invalid: %v", ErrInvalidConfig, t.Name, err)
			}
		}
	}

	// Validate Env
	for _, e := range cfg.Env {
		if len(e.Name) == 0 || len(e.Name) > maxEnvNameBytes {
			return cfg, fmt.Errorf("%w: env var name must be 1-%d bytes", ErrInvalidConfig, maxEnvNameBytes)
		}
	}

	// Validate Files
	for _, f := range cfg.Files {
		if strings.HasPrefix(f, "/") || strings.HasPrefix(f, "\\") {
			return cfg, fmt.Errorf("%w: file path cannot be absolute: %q", ErrInvalidConfig, f)
		}
		cleaned := path.Clean(strings.ReplaceAll(f, "\\", "/"))
		if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			return cfg, fmt.Errorf("%w: file path cannot contain '..': %q", ErrInvalidConfig, f)
		}
	}

	return cfg, nil
}

func formatWant(min, max string) string {
	if min != "" && max != "" {
		return ">=" + min + " <" + max
	}
	if min != "" {
		return ">=" + min
	}
	if max != "" {
		return "<" + max
	}
	return ""
}

// Check executes all environment validations using the provided dependencies.
func Check(ctx context.Context, cfg Config, deps Deps) Report {
	report := Report{
		OK:      true,
		Results: make([]Result, 0, len(cfg.Tools)+len(cfg.Env)+len(cfg.Files)),
	}

	// 1. Tools
	for _, t := range cfg.Tools {
		res := checkTool(ctx, t, deps.Runner)
		if res.Status != StatusOK && !t.Optional {
			report.OK = false
		}
		report.Results = append(report.Results, res)
	}

	// 2. Env
	for _, e := range cfg.Env {
		res := checkEnv(e, deps.Getenv)
		if res.Status != StatusOK && !res.Optional {
			report.OK = false
		}
		report.Results = append(report.Results, res)
	}

	// 3. Files
	for _, f := range cfg.Files {
		res := checkFile(f, deps.FS)
		if res.Status != StatusOK && !res.Optional {
			report.OK = false
		}
		report.Results = append(report.Results, res)
	}

	return report
}

func checkTool(ctx context.Context, t Tool, runner Runner) Result {
	res := Result{
		Kind:     "tool",
		Name:     t.Name,
		Want:     formatWant(t.Min, t.Max),
		Optional: t.Optional,
	}

	if runner == nil {
		res.Status = StatusError
		return res
	}

	timeout := time.Duration(t.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Duration(defaultTimeoutMS) * time.Millisecond
	}
	ctxTool, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmdName := t.Command[0]
	cmdArgs := t.Command[1:]

	out, err := runner.Run(ctxTool, cmdName, cmdArgs...)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			res.Status = StatusMissing
			return res
		}
		if errors.Is(ctxTool.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			res.Status = StatusTimeout
			return res
		}
		res.Status = StatusError
		return res
	}

	re := t.patternRe
	if re == nil {
		var compErr error
		re, compErr = regexp.Compile(t.Pattern)
		if compErr != nil {
			res.Status = StatusError
			return res
		}
	}

	match := re.FindSubmatch(out)
	if len(match) < 2 {
		res.Status = StatusUnknownVersion
		return res
	}

	foundVer := string(match[1])
	res.Found = foundVer

	if t.Min != "" {
		cmp, err := CompareVersions(foundVer, t.Min)
		if err != nil || cmp < 0 {
			res.Status = StatusOutdated
			return res
		}
	}

	if t.Max != "" {
		cmp, err := CompareVersions(foundVer, t.Max)
		if err != nil || cmp >= 0 {
			res.Status = StatusUnsupported
			return res
		}
	}

	res.Status = StatusOK
	return res
}

func checkEnv(e EnvVar, getenv func(string) (string, bool)) Result {
	res := Result{
		Kind:     "env",
		Name:     e.Name,
		Optional: !e.Required,
	}

	if getenv == nil {
		res.Status = StatusMissingEnv
		return res
	}

	val, ok := getenv(e.Name)
	if !ok {
		if e.Required {
			res.Status = StatusMissingEnv
		} else {
			res.Status = StatusMissingEnv
		}
		return res
	}

	res.Status = StatusOK
	res.Found = val
	return res
}

func checkFile(filePath string, fsys fs.FS) Result {
	res := Result{
		Kind:     "file",
		Name:     filePath,
		Optional: false,
	}

	if fsys == nil {
		res.Status = StatusMissingFile
		return res
	}

	cleanPath := path.Clean(strings.ReplaceAll(filePath, "\\", "/"))
	cleanPath = strings.TrimPrefix(cleanPath, "./")

	_, err := fs.Stat(fsys, cleanPath)
	if err != nil {
		res.Status = StatusMissingFile
		return res
	}

	res.Status = StatusOK
	res.Found = filePath
	return res
}

// execRunner implements Runner using os/exec.
type execRunner struct{}

// NewExecRunner returns a Runner that invokes external system processes.
func NewExecRunner() Runner {
	return &execRunner{}
}

func (r *execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, exec.ErrDot) {
			return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
		}
		return nil, err
	}

	cmd := exec.CommandContext(ctx, path, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err = cmd.Run()
	out := buf.Bytes()
	if len(out) > maxOutputBytes {
		out = out[:maxOutputBytes]
	}

	if err != nil {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, exec.ErrDot) {
			return out, fmt.Errorf("%w: %v", ErrNotFound, err)
		}
		return out, err
	}
	return out, nil
}

// FormatText writes report results as tab-separated values.
// Format: status\tname\tfound\twant\n
func FormatText(w io.Writer, rep Report) error {
	for _, r := range rep.Results {
		found := r.Found
		if found == "" {
			found = "-"
		}
		want := r.Want
		if want == "" {
			want = "-"
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Status, r.Name, found, want); err != nil {
			return err
		}
	}
	return nil
}

// FormatJSON writes report results as indented JSON.
func FormatJSON(w io.Writer, rep Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

// Run coordinates CLI arguments and writes diagnostic output.
// Returns 0 on success, 1 on check failure, and 2 on configuration error.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("dev-env-doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)

	configPath := fs.String("config", "devenv.json", "path to JSON configuration file")
	format := fs.String("format", "text", "output format: text|json")
	root := fs.String("root", ".", "fs root directory")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	f, err := os.Open(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "Error opening config %q: %v\n", *configPath, err)
		return 2
	}
	defer f.Close()

	cfg, err := LoadConfig(f)
	if err != nil {
		fmt.Fprintf(stderr, "Error loading config: %v\n", err)
		return 2
	}

	deps := Deps{
		Runner: NewExecRunner(),
		Getenv: os.LookupEnv,
		FS:     os.DirFS(*root),
	}

	rep := Check(context.Background(), cfg, deps)

	switch *format {
	case "json":
		if err := FormatJSON(stdout, rep); err != nil {
			fmt.Fprintf(stderr, "Error writing JSON: %v\n", err)
			return 2
		}
	default:
		if err := FormatText(stdout, rep); err != nil {
			fmt.Fprintf(stderr, "Error writing text: %v\n", err)
			return 2
		}
	}

	if rep.OK {
		return 0
	}
	return 1
}
