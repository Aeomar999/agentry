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
	"gitlab.com/dev.chempa/agentry/internal/pinned"
)

// `agentry send "message"` is how Claude, in a task's Workspace
// (machines.Helper), sends the coordinator a message you asked it to send.
// It goes as yours, as if typed in the chat. Claude Code asks you before it
// runs this, so Claude never sends one on its own. Its token does nothing else.
func runSend(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	server := strings.TrimSuffix(os.Getenv(hookcall.EnvServer), "/")
	token, task := os.Getenv(machines.EnvHelperToken), os.Getenv(hookcall.EnvTask)
	if server == "" || token == "" || task == "" {
		fmt.Fprintf(stderr, "agentry send: run it in the Claude session you opened from a task's Workspace (%s, %s and %s are unset)\n",
			hookcall.EnvServer, machines.EnvHelperToken, hookcall.EnvTask)
		return 2
	}
	text := strings.Join(args, " ")
	if strings.TrimSpace(text) == "" {
		read, _ := io.ReadAll(stdin)
		text = string(read)
	}
	if strings.TrimSpace(text) == "" {
		fmt.Fprintln(stderr, `usage: agentry send "what the coordinator should do"   (or the message on stdin)`)
		return 2
	}
	body, _ := json.Marshal(map[string]string{"text": text})
	request, _ := http.NewRequest(http.MethodPost, server+"/api/tasks/"+task+"/claude/message", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := pinned.Client([]byte(os.Getenv(hookcall.EnvCA)), 15*time.Second).Do(request)
	if err != nil {
		fmt.Fprintln(stderr, "agentry send: no answer from the Agentry server:", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		reason, _ := io.ReadAll(response.Body)
		fmt.Fprintf(stderr, "agentry send: %s\n", strings.TrimSpace(string(reason)))
		return 1
	}
	fmt.Fprintln(stdout, "Sent to the coordinator. It reads it now, and the task's chat shows it.")
	return 0
}
