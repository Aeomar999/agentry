package tlsca

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTheCAIsMadeOnceAndReused(t *testing.T) {
	dir := filepath.Join(t.TempDir(), Folder)
	first, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	hostname, _ := os.Hostname()
	cert := first.cert
	if cert.Subject.CommonName != "Agentry local CA ("+hostname+")" {
		t.Errorf("CN = %q", cert.Subject.CommonName)
	}
	if !cert.IsCA || !cert.MaxPathLenZero || cert.MaxPathLen != 0 {
		t.Errorf("IsCA %v, MaxPathLen %d (zero %v)", cert.IsCA, cert.MaxPathLen, cert.MaxPathLenZero)
	}
	if years := cert.NotAfter.Sub(cert.NotBefore).Hours() / 24 / 365; years < 9.9 || years > 10.1 {
		t.Errorf("lifetime %.1f years", years)
	}
	if runtime.GOOS != "windows" {
		for path, want := range map[string]os.FileMode{dir: 0o700, filepath.Join(dir, keyFile): 0o600} {
			if info, err := os.Stat(path); err != nil || info.Mode().Perm() != want {
				t.Errorf("%s: %v, %v; want %v", path, info.Mode().Perm(), err, want)
			}
		}
	}

	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if again.Fingerprint() != first.Fingerprint() || !again.key.Equal(first.key) {
		t.Error("a second Open made a new CA")
	}
}

func TestAKeyOthersMayReadIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows keeps no Unix permissions")
	}
	dir := t.TempDir()
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []os.FileMode{0o644, 0o640, 0o604} {
		os.Chmod(filepath.Join(dir, keyFile), mode)
		if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "chmod 600") {
			t.Errorf("mode %v: %v", mode, err)
		}
	}
	os.Chmod(filepath.Join(dir, keyFile), 0o400)
	if _, err := Open(dir); err != nil {
		t.Errorf("a key only its owner reads: %v", err)
	}
}

func TestAKeyThatIsALinkIsRefused(t *testing.T) {
	dir, elsewhere := t.TempDir(), t.TempDir()
	if _, err := Open(elsewhere); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{keyFile, certFile} {
		data, _ := os.ReadFile(filepath.Join(elsewhere, name))
		os.WriteFile(filepath.Join(dir, name), data, 0o600)
	}
	os.Remove(filepath.Join(dir, keyFile))
	if err := os.Symlink(filepath.Join(elsewhere, keyFile), filepath.Join(dir, keyFile)); err != nil {
		t.Skip("no symbolic links here:", err)
	}
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("a linked key: %v", err)
	}
}

func TestAKeyThatIsNotAFileIsRefused(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(dir, keyFile))
	os.Mkdir(filepath.Join(dir, keyFile), 0o700)
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "not a plain file") {
		t.Errorf("a folder for a key: %v", err)
	}
}

func TestAFolderThatIsALinkOrAFileIsRefused(t *testing.T) {
	elsewhere := t.TempDir()
	if _, err := Open(elsewhere); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(t.TempDir(), Folder)
	if err := os.Symlink(elsewhere, linked); err != nil {
		t.Skip("no symbolic links here:", err)
	}
	if _, err := Open(linked); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("a linked folder: %v", err)
	}

	file := filepath.Join(t.TempDir(), Folder)
	os.WriteFile(file, nil, 0o600)
	if _, err := Open(file); err == nil || !strings.Contains(err.Error(), "not a folder") {
		t.Errorf("a file for a folder: %v", err)
	}
}

func TestAFolderOpenToOthersIsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows keeps no Unix permissions")
	}
	dir := filepath.Join(t.TempDir(), Folder)
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	os.Chmod(dir, 0o755)
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(dir); info.Mode().Perm() != 0o700 {
		t.Errorf("the folder is %v after Open, want 0700", info.Mode().Perm())
	}
}

func TestAHalfMadeOrMismatchedCAIsRefused(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(dir, certFile))
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Errorf("a key with no certificate: %v", err)
	}

	other := t.TempDir()
	if _, err := Open(other); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(other, certFile))
	os.WriteFile(filepath.Join(dir, certFile), data, 0o644)
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "doesn't belong") {
		t.Errorf("another CA's certificate: %v", err)
	}
}

func TestRegenerateMakesANewCA(t *testing.T) {
	dir := t.TempDir()
	old, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Regenerate(dir); err != nil {
		t.Fatal(err)
	}
	fresh, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Fingerprint() == old.Fingerprint() {
		t.Error("the same CA after Regenerate")
	}
	if err := Regenerate(t.TempDir()); err != nil {
		t.Errorf("regenerating where there is no CA: %v", err)
	}
}

func TestTheCertificateIsHandedOutInEachForm(t *testing.T) {
	ca, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(ca.PEM()), "-----BEGIN CERTIFICATE-----\n") || strings.Contains(string(ca.PEM()), "PRIVATE") {
		t.Errorf("PEM:\n%s", ca.PEM())
	}
	sum := sha256.Sum256(ca.DER())
	want := strings.ToUpper(hex.EncodeToString(sum[:]))
	if got := strings.ReplaceAll(ca.Fingerprint(), ":", ""); got != want || len(ca.Fingerprint()) != 32*3-1 {
		t.Errorf("Fingerprint = %s, want the SHA-256 %s", ca.Fingerprint(), want)
	}
}
