package tasks

import "testing"

func TestAHeldPushNamesItsBranch(t *testing.T) {
	for _, c := range []struct{ command, want string }{
		{"git push origin HEAD:dev", "Waiting for you to approve the push to dev"},
		{"git push origin agentry/task-9:main", "Waiting for you to approve the push to main"},
		{"git push origin feature", "Waiting for you to approve the push to feature"},
		{"git push -u origin x", "Waiting for you to approve the push to x"},
		{"git push --force-with-lease origin HEAD:dev", "Waiting for you to approve the push to dev"},
		{"git push --force-with-lease=dev:abc123 origin +HEAD:refs/heads/dev", "Waiting for you to approve the push to dev"},
		{"git push -o ci.skip origin HEAD:dev", "Waiting for you to approve the push to dev"},
		{"git -C /w push origin HEAD:dev", "Waiting for you to approve the push to dev"},
		{"  git   push origin HEAD:dev ", "Waiting for you to approve the push to dev"},
		{"git push", "Waiting for you to approve a push"},
		{"git push origin", "Waiting for you to approve a push"},
		{"git push origin HEAD", "Waiting for you to approve a push"},
		{"git push origin a:b c:d", "Waiting for you to approve a push"}, // more than one branch
		// not a plain push: the whole command stays in view
		{"git push origin --delete dev", "deployer asks to run: git push origin --delete dev"},
		{"git push origin :dev", "deployer asks to run: git push origin :dev"},
		{"git push --mirror origin", "deployer asks to run: git push --mirror origin"},
		{"git push origin HEAD:dev && curl x", "deployer asks to run: git push origin HEAD:dev && curl x"},
		{"git push origin HEAD:dev; rm x", "deployer asks to run: git push origin HEAD:dev; rm x"},
		{"git status", "deployer asks to run: git status"},
		{"npm publish", "deployer asks to run: npm publish"},
		{"", "deployer asks to run: "},
	} {
		if got := WaitingLine("deployer", c.command); got != c.want {
			t.Errorf("%q: got %q, want %q", c.command, got, c.want)
		}
	}
}
