package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMainWrapper(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "devenv.json")
	if err := os.WriteFile(cfgPath, []byte(`{"tools":[],"env":[],"files":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	oldArgs := os.Args
	oldExit := exitFunc
	defer func() {
		os.Args = oldArgs
		exitFunc = oldExit
	}()

	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
	}

	os.Args = []string{"dev-env-doctor", "--config", cfgPath}
	main()

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
}
