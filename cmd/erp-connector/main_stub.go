//go:build !windows

package main

import (
	"fmt"
	"log"
	"os"
)

// main on non-Windows supports only headless/CLI mode.
func main() {
	uiLog := newUILogger()
	defer uiLog.Close()

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetOutput(uiLog.Writer())

	if handled, err := runHeadless(uiLog); handled {
		if err != nil {
			uiLog.Printf("headless error: %v", err)
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		return
	}

	fmt.Fprintln(os.Stderr, "erp-connector GUI requires Windows. Use --headless for CLI mode.")
	os.Exit(1)
}
