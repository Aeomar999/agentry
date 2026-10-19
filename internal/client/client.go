// Package client calls a running Agentry server's API, for the commands
// that run beside it: the desk's `agentry task`, `agentry mcp` for Claude
// Code, and the status line.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/pinned"
)

// ServerFile is where `agentry serve` writes how to reach it, in its data
// folder, so the commands on this machine find it with nothing set.
const ServerFile = "server.json"

// Where is how a server is reached: URL for the API, Page for links people
// open (the public address, when it has one), Listen for the address it
// listens on, which says whether other machines reach it. HTTPS says it is
// served over https with Agentry's own CA, and CA is that CA's certificate,
// PEM encoded, which the client trusts instead of the system's store.
type Where struct {
	URL    string `json:"url"`
	Page   string `json:"page"`
	Listen string `json:"listen,omitempty"`
	HTTPS  bool   `json:"https,omitempty"`
	CA     string `json:"ca,omitempty"`
}

// Save writes where the server is, in its data folder.
func Save(dataDir string, where Where) error {
	data, _ := json.MarshalIndent(where, "", "  ")
	return os.WriteFile(filepath.Join(dataDir, ServerFile), append(data, '\n'), 0o600)
}

// Connection is a server this machine's Claude Code was connected to from
// its Integrations page, and the credential it got: kept, private, in the
// user's config folder. CA is the server's own CA, PEM encoded, when it is
// served over https with one.
type Connection struct {
	URL        string `json:"url"`
	Credential string `json:"credential"`
	CA         string `json:"ca,omitempty"`
}

// ConnectionFile is where the connection is kept.
func ConnectionFile() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "agentry", "connection.json")
}

// SaveConnection keeps the connection, readable only by you.
func SaveConnection(c Connection) error {
	path := ConnectionFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// Connected is the server this machine was connected to, if it was.
func Connected() (Client, bool) {
	data, err := os.ReadFile(ConnectionFile())
	if err != nil {
		return Client{}, false
	}
	var c Connection
	if json.Unmarshal(data, &c) != nil || c.URL == "" || c.Credential == "" {
		return Client{}, false
	}
	return Client{Where: Where{URL: c.URL, Page: c.URL, CA: c.CA}, Token: c.Credential}, true
}

// Client calls one server with one token.
type Client struct {
	Where
	Token   string
	Timeout time.Duration // 0 is 15 seconds
	// Untrusted, when set, is given the error when the server's certificate
	// isn't one CA issued, as when the CA was kept from an earlier check
	// and the server's has since been regenerated, and returns the error to
	// report, which can say what to do about it.
	Untrusted func(error) error
}

// Local is the server running on this machine, from its data folder: the
// address it saved and its page's token.
func Local(dataDir string) (Client, error) {
	saved, err := os.ReadFile(filepath.Join(dataDir, ServerFile))
	if err != nil {
		return Client{}, fmt.Errorf("no Agentry server found in %s; start one with `agentry serve`, or give its address", dataDir)
	}
	var where Where
	if err := json.Unmarshal(saved, &where); err != nil || where.URL == "" {
		return Client{}, fmt.Errorf("%s is unreadable", filepath.Join(dataDir, ServerFile))
	}
	token, err := os.ReadFile(filepath.Join(dataDir, "ui-token"))
	if err != nil {
		return Client{}, fmt.Errorf("no page token in %s", dataDir)
	}
	return Client{Where: where, Token: strings.TrimSpace(string(token))}, nil
}

// Call sends body as JSON and reads the reply into into, when into isn't
// nil. A refusal is an error with the server's reason.
func (c Client) Call(method, path string, body, into any) error {
	return c.CallContext(context.Background(), method, path, body, into)
}

// CallContext is Call that gives up when ctx ends.
func (c Client) CallContext(ctx context.Context, method, path string, body, into any) error {
	var payload io.Reader
	if body != nil {
		encoded, _ := json.Marshal(body)
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(c.URL, "/")+path, payload)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.Token)
	request.Header.Set("Content-Type", "application/json")
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	response, err := c.HTTP(timeout).Do(request)
	if err != nil {
		unreachable := fmt.Errorf("can't reach Agentry at %s: %w", c.URL, err)
		if c.Untrusted != nil && pinned.Untrusted(err) {
			return c.Untrusted(unreachable)
		}
		return unreachable
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s", strings.TrimSpace(string(content)))
	}
	if into == nil {
		return nil
	}
	return json.Unmarshal(content, into)
}

// HTTP is an HTTP client for the server, trusting only its CA when it has
// one. It gives up after timeout, or never for 0.
func (c Client) HTTP(timeout time.Duration) *http.Client {
	return pinned.Client([]byte(c.CA), timeout)
}

// PageURL is the link to a page of Agentry, such as "/tasks/task-4".
func (c Client) PageURL(path string) string {
	page := c.Page
	if page == "" {
		page = c.URL
	}
	return strings.TrimSuffix(page, "/") + path
}
