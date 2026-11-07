package main

import (
	"os"

	"github.com/Trysac/u175-dev-env-doctor"
)

var exitFunc = os.Exit

func main() {
	code := devenvdoctor.Run(os.Args[1:], os.Stdout, os.Stderr)
	exitFunc(code)
}
