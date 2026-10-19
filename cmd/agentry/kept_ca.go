package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/client"
	"gitlab.com/dev.chempa/agentry/internal/pinned"
	"gitlab.com/dev.chempa/agentry/internal/tlsca"
)

// The hooks and the status line run with every prompt Claude Code takes,
// so a server's CA they checked by fingerprint is kept, beside the saved
// connection, and the next run trusts it with no request at all.

// envTrustTimeout is how long the hooks, the status line and `agentry wait`
// wait for a trust check: about what they give the server to answer them
// at all.
const envTrustTimeout = 3 * time.Second

// keptCA is the CA kept for the server as it was given (AGENTRY_URL). The
// address it is trusted at is worked out from that, never read from the
// file, so a file someone else wrote can't send the token elsewhere.
type keptCA struct {
	Given string `json:"given"`
	CA    string `json:"ca"`
}

func keptCAFile() string {
	return filepath.Join(filepath.Dir(client.ConnectionFile()), "trusted-ca.json")
}

// trustServerOnce is trustServer with a short timeout, the CA kept once it
// is checked. A kept CA is trusted only for the same server and only when
// it has the fingerprint given.
func trustServerOnce(server, fingerprint string) (trustedServer, error) {
	if fingerprint == "" {
		return trustServer(server, "", envTrustTimeout)
	}
	given := strings.TrimSuffix(server, "/")
	if kept, ok := readKeptCA(given, fingerprint); ok {
		return kept, nil
	}
	trusted, err := trustServer(server, fingerprint, envTrustTimeout)
	if err != nil {
		return trustedServer{}, err
	}
	keepCA(keptCA{Given: given, CA: string(trusted.ca)}) // if it can't be kept, it is checked again next time
	return trusted, nil
}

func readKeptCA(given, fingerprint string) (trustedServer, bool) {
	data, err := os.ReadFile(keptCAFile())
	if err != nil {
		return trustedServer{}, false
	}
	var kept keptCA
	if json.Unmarshal(data, &kept) != nil || kept.Given != given {
		return trustedServer{}, false
	}
	ca := []byte(kept.CA)
	if pinned.Check(ca) != nil || normalFingerprint(tlsca.FingerprintOf(derOf(ca))) != normalFingerprint(fingerprint) {
		return trustedServer{}, false
	}
	return trustedServer{server: httpsOf(given), ca: ca}, true
}

// keepCA writes the CA as the connection is written: readable only by you.
// It is written whole beside the file and renamed into place, so hooks
// running at once never read half of it.
func keepCA(kept keptCA) error {
	path := keptCAFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(kept, "", "  ")
	temp, err := os.CreateTemp(filepath.Dir(path), ".trusted-ca-*.json") // made 0600
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name()) // gone once renamed
	_, err = temp.Write(append(data, '\n'))
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}

// forgetKeptCA drops the kept CA, so the next run checks the server again.
func forgetKeptCA() {
	os.Remove(keptCAFile())
}
