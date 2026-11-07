// Package devenvdoctor is a small, dependency-free Go library: Diagnose a development environment from a JSON config: required tools and version ranges, environment variables and files, with an injectable command runner and CI exit codes
//
// The public API is every exported identifier of this root package. Functions are pure: they do
// no I/O, keep no global mutable state and report invalid input through returned errors.
package devenvdoctor
