# dev-env-doctor

New contributors lose hours to the wrong tool versions, missing environment variables, or missing files. Existing onboarding guides are frequently outdated, and custom verification scripts are often platform-specific and parse version output inaccurately. `dev-env-doctor` provides a portable, dependency-free verification engine that validates development environments against a declarative JSON configuration, ensuring deterministic outcomes for both CI and local developer setups.

## Installation

```bash
go install github.com/Trysac/u175-dev-env-doctor/cmd/dev-env-doctor@latest
```

## Usage

```go
package main

import (
	"context"
	"os"
	"strings"
	"github.com/Trysac/u175-dev-env-doctor"
)

func main() {
	cfg, _ := devenvdoctor.LoadConfig(strings.NewReader(jsonConfig)) // Load from reader
	ctx := context.Background()
	results := devenvdoctor.Check(ctx, cfg, devenvdoctor.Deps{
		Runner: devenvdoctor.NewExecRunner(),
		Getenv: os.LookupEnv,
		FS:     os.DirFS("."),
	})
}
```

## Public API Summary

- `LoadConfig(r io.Reader) (Config, error)`: Parses JSON, validates schema, and compiles regex patterns.
- `Check(ctx context.Context, cfg Config, deps Deps) Report`: Executes verification logic based on injected dependencies.
- `CompareVersions(a, b string) (int, error)`: Compares SemVer-like strings (1-3 segments).
- `NewExecRunner() Runner`: Returns the production `Runner` implementation using `os/exec`.
- **Errors**: `ErrNotFound`, `ErrInvalidVersion`, `ErrInvalidConfig` are exported for error handling.

## Limits and Options

- **Config Size**: Limited to 1 MiB.
- **Output Buffering**: 64 KiB limit for command output.
- **Tool Execution**: Per-tool `timeout_ms` (default 5000ms).
- **Formats**: Supports `text` and `json` outputs.

## Why not manual scripts?

Unlike ad-hoc shell scripts, `dev-env-doctor` provides a portable binary that enforces strict, exact version comparisons (preventing loose parsing bugs), maintains a consistent exit code convention for CI, and provides machine-readable diagnostic reports.

## Security

See [docs/SECURITY.md](docs/SECURITY.md) for the threat model and limits.