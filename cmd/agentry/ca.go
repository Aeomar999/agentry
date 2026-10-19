package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gitlab.com/dev.chempa/agentry/internal/tlsca"
)

// `agentry ca` shows the fingerprint of Agentry's own CA, which `serve
// -https` serves with, so a device's copy can be checked against it.
// `agentry ca -regenerate` replaces the CA, as when its key may have leaked.
// A running server keeps the CA it loaded, so regenerating refuses while
// the server for that data folder answers.

func runCA(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	home, _ := os.UserHomeDir()
	flags := flag.NewFlagSet("ca", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dataDir := flags.String("data", filepath.Join(home, ".agentry"), "where Agentry keeps its log, the worktrees and the CA")
	regenerate := flags.Bool("regenerate", false, "replace the CA with a new one; every device must then trust the new one")
	if flags.Parse(args) != nil {
		return 2
	}
	dir := tlsca.Dir(*dataDir)
	if *regenerate {
		if _, running := runningServer(*dataDir); running {
			fmt.Fprintln(stderr, "agentry ca: Agentry is running from", *dataDir+", and keeps the CA it loaded. Stop it first (agentry stop, or Ctrl-C where it runs), then regenerate.")
			return 1
		}
		if err := tlsca.Regenerate(dir); err != nil {
			fmt.Fprintln(stderr, "agentry ca:", err)
			return 1
		}
	}
	ca, err := tlsca.Open(dir)
	if err != nil {
		fmt.Fprintln(stderr, "agentry ca:", err)
		return 1
	}
	if *regenerate {
		fmt.Fprintln(stdout, "Made a new CA. Devices that trusted the old one no longer trust this server: on each, remove the old Agentry local CA and trust the new one from /trust/.")
		fmt.Fprintln(stdout, "Workers and connected Claude Code machines must join again with the new fingerprint below: run the install command again, "+
			"with -ca-fingerprint. Claude Code sessions open now can't reach Agentry until they are restarted, after joining again.")
	}
	fmt.Fprintf(stdout, "Agentry's CA in %s, SHA-256 %s\n", dir, ca.Fingerprint())
	return 0
}
