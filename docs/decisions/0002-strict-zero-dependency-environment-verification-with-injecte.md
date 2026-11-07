# Strict zero-dependency environment verification with injected runner abstraction

Status: accepted (generated, pending human review)

## Context

Development teams frequently use disparate shell scripts to check prerequisites, leading to cross-platform incompatibilities and non-deterministic version checks.

## Decision

Implement devenvdoctor entirely within Go standard library, providing an injectable Runner interface, standard io/fs file verification, and strict decimal version comparison.

## Consequences

Eliminates third-party dependency vulnerabilities, guarantees deterministic unit tests without real tool installations, and ensures predictable exit codes across POSIX and Windows systems.
