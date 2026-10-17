package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/machines"
)

// `agentry draft "message"` is how the Claude session you open beside a
// task (machines.Helper) puts a message for the coordinator in front of you.
// It only drafts: the message reaches the coordinator when you send it from
// the page, because it would start work. Its token does nothing else.
func runDraft(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	server := strings.TrimSuffix(os.Getenv(hookcall.EnvServer), "/")
	token, task := os.Getenv(machines.EnvHelperToken), os.Getenv(hookcall.EnvTask)
	if server == "" || token == "" || task == "" {
		fmt.Fprintf(stderr, "agentry draft: run it in the Claude session you opened from a task's Workspace (%s, %s and %s are unset)\n",
			hookcall.EnvServer, machines.EnvHelperToken, hookcall.EnvTask)
		return 2
	}
	text := strings.Join(args, " ")
	if strings.TrimSpace(text) == "" {
		read, _ := io.ReadAll(stdin)
		text = string(read)
	}
	if strings.TrimSpace(text) == "" {
		fmt.Fprintln(stderr, `usage: agentry draft "what the coordinator should do"   (or the message on stdin)`)
		return 2
	}
	body, _ := json.Marshal(map[string]string{"text": text})
	request, _ := http.NewRequest(http.MethodPost, server+"/api/tasks/"+task+"/drafts", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		fmt.Fprintln(stderr, "agentry draft: no answer from the Agentry server:", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reason, _ := io.ReadAll(response.Body)
		fmt.Fprintf(stderr, "agentry draft: %s\n", strings.TrimSpace(string(reason)))
		return 1
	}
	fmt.Fprintln(stdout, "Drafted for the user to review. It reaches the coordinator only if they send it from the page.")
	return 0
}
