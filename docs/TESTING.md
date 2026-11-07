# Testing Strategy

`dev-env-doctor` employs unit and integration testing focused on deterministic environment validation without external side effects.

## Test Principles

- **Mocking**: Testing relies on dependency injection. We use a `fakeRunner` to simulate command output and timeouts, a map-based `Getenv` function for environment variables, and `fstest.MapFS` for filesystem inspection.
- **Process Verification**: The production `ExecRunner` is verified using the Go helper-process pattern, where tests use `os.Args[0]` with an environment flag to simulate external binary behavior rather than executing real system utilities.
- **Coverage**: Tests cover:
    - **Version Logic**: Validates `CompareVersions` logic against expected outcomes (`1.22` == `1.22.0`, `1.9` < `1.10`, etc.) and `ErrInvalidVersion` handling.
    - **Config Validation**: Ensures `LoadConfig` rejects invalid schemas, missing capture groups in regex, and configurations larger than the 1 MiB limit.
    - **Timeout Behavior**: Validates that tools exceeding their `TimeoutMS` result in a `timeout` status.
    - **Reporting**: Ensures that `Check` produces correct status flags (`ok`, `missing`, `outdated`, `unsupported`, etc.) and maintains the correct order of operations (Tools -> Env -> Files).