//go:build !desktop

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "agentry-desktop: build it with `wails build`, or go build -tags desktop,production (and webkit2_41 on Linux)")
	os.Exit(2)
}
