# Architecture

`dev-env-doctor` provides a portable, dependency-free development environment verification engine.

## Core Components

1. **Configuration (`Config`, `LoadConfig`)**:
   Parses and strictly validates JSON definitions. Rejects unknown keys, invalid regular expressions without capture groups, invalid version formats, and path traversal attempts in file lists.

2. **Version Comparison (`CompareVersions`)**:
   Evaluates 1 to 3 numeric decimal parts with up to 9 digits each. Treats absent sub-versions as 0 (`1.22` == `1.22.0`).

3. **Runner Interface (`Runner`, `ExecRunner`)**:
   Decouples command execution from system calls. Production runs use `os/exec` with 64 KiB buffer limits, while tests use mock runners.

4. **Check Engine (`Check`)**:
   Evaluates tools, environment variables, and filesystem entries in a deterministic sequence, returning a unified `Report`.
