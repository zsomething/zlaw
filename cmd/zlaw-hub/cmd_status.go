package main

import (
	"flag"
	"fmt"
	"os"
)

// runStatus prints a stub hub status.
// Full implementation is wired in Phase 2 once the supervisor exists.
func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: zlaw-hub status")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	fmt.Println("Hub status: not running")
	fmt.Println("(Phase 2 will query the supervisor process via Unix socket)")
	return nil
}
