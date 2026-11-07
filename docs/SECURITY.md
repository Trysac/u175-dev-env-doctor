# Security and Threat Model

## Threat Model

- **Denial of Service (Configuration)**: The `LoadConfig` function utilizes `io.LimitReader` to strictly reject configuration inputs exceeding 1 MiB.
- **Resource Exhaustion**: `ExecRunner` caps command stdout/stderr buffers at 64 KiB to prevent memory exhaustion.
- **Command Injection**: `ExecRunner` executes commands directly via `os/exec.CommandContext` without invoking shell interpolation (i.e., it does not use `sh -c` or `cmd.exe /c`), preventing shell injection attacks.
- **Process Control**: Every tool execution is wrapped in a `context.Context` with a configurable timeout (default 5,000 ms) to prevent hanging processes.
- **Path Traversal**: `LoadConfig` validates file paths, rejecting any paths that use absolute references or directory traversal patterns (`..`).