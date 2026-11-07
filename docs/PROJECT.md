# Project record

## Objective

To provide a robust, cross-platform diagnostic utility that verifies development environments via declarative JSON configurations, replacing fragile shell scripts with a structured Go library and CLI tool.

## Scope

- Parsing and strict validation of `devenv.json` configurations.
- Verification of CLI tool presence and version constraints using standard SemVer-like logic.
- Verification of required environment variables.
- Verification of required filesystem entries.
- Dependency injection for `Runner`, `Getenv`, and `fs.FS` to enable deterministic testing.
- Machine-readable status reporting (JSON/TSV) and standard CI exit codes.

## Out of Scope

- Installing or upgrading missing tools (the utility is diagnostic only).
- Inspecting shell profile files (e.g., `.bashrc`, `.zshrc`).
- Managing system-level package managers (e.g., `apt`, `brew`).
- Performing network connectivity checks.

## Timeline

| Date | Event |
|---|---|
| 2026 | Repository created from the factory utility template. |

## Effort metrics

| Metric | Value |
|---|---|
| Tokens (input / output) | 719487 / 20720 |
| Hours (human) | pending measurement |
| Cost (USD) | 0.1577 |

## Factory run u175 (measured from API usage)

| Phase | Model | Batch | Input | Output | Cache write | Cache read | Cost (USD) |
|---|---|---|---|---|---|---|---|
| generation | gemini-3.8-flash | yes | 4413 | 16816 | 0 | 0 | 0.0332 |
| fix-1 | gemini-3.8-flash | no | 83328 | 2265 | 0 | 555799 | 0.1127 |
| audit | gemini-3.1-flash-lite | no | 29063 | 107 | 0 | 42809 | 0.0085 |
| docs | gemini-3.1-flash-lite | no | 4075 | 1532 | 0 | 0 | 0.0033 |

Total: $0.1577. Prices: Anthropic list prices.
