package workerlink

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/pinned"
	"gitlab.com/dev.chempa/agentry/internal/tlsca"
)

// A worker pinned to a CA the server no longer has keeps dialling, and
// says once, not on every dial, how to join again.
func TestAWorkerWhoseCAChangedSaysOnceHowToJoinAgain(t *testing.T) {
	server := httptest.NewUnstartedServer(http.NotFoundHandler())
	server.Config.ErrorLog = log.New(io.Discard, "", 0) // the refused handshakes
	server.StartTLS()                                   // with another CA's certificate
	defer server.Close()
	old, err := tlsca.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var logged []string
	worker := &Worker{Server: server.URL, Credential: "c1", HTTP: pinned.Client(old.PEM(), 0),
		Log: func(format string, args ...any) {
			mu.Lock()
			defer mu.Unlock()
			logged = append(logged, fmt.Sprintf(format, args...))
		}}
	ctx, cancel := context.WithTimeout(context.Background(), 3500*time.Millisecond) // dials at 0, 1 and 3 s
	defer cancel()
	worker.Run(ctx)

	mu.Lock()
	defer mu.Unlock()
	told, down := 0, 0
	for _, line := range logged {
		if strings.Contains(line, "agentry worker join -ca-fingerprint") {
			told++
		}
		if strings.HasPrefix(line, "link down") {
			down++
		}
	}
	if told != 1 || down < 2 {
		t.Errorf("told %d times over %d dials, want once:\n%s", told, down, strings.Join(logged, "\n"))
	}
}
