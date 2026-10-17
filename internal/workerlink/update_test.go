package workerlink

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A worker running an older agentry than the server's takes the server's
// in its place; one running the same takes nothing.
func TestAWorkerUpdatesToTheServersAgentry(t *testing.T) {
	latest := []byte("agentry v2")
	sum := sha256.Sum256(latest)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != UpdatePath || r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:])+`"`)
		http.ServeContent(w, r, "agentry", time.Time{}, bytes.NewReader(latest))
	}))
	defer server.Close()

	self := filepath.Join(t.TempDir(), "agentry")
	os.WriteFile(self, []byte("agentry v1"), 0o755)
	w := &Worker{Server: server.URL, Credential: "secret"}
	if updated, err := w.replace(context.Background(), self); err != nil || !updated {
		t.Fatalf("an old agentry wasn't updated: %v %v", updated, err)
	}
	if content, _ := os.ReadFile(self); !bytes.Equal(content, latest) {
		t.Errorf("after the update it is %q", content)
	}
	if info, _ := os.Stat(self); info.Mode().Perm()&0o100 == 0 {
		t.Error("the new agentry can't be run")
	}
	if updated, err := w.replace(context.Background(), self); err != nil || updated {
		t.Errorf("a current agentry was replaced again: %v %v", updated, err)
	}
}
