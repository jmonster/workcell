package main

import (
	"fmt"
	"os"

	"github.com/jmonster/workcell/internal/workcell"
)

func main() {
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "workcell: resolve executable: %v\n", err)
		os.Exit(workcell.ExitInternal)
	}
	os.Exit(workcell.Main(executable, os.Args[1:], os.Stdout, os.Stderr))
}
