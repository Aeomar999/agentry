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
		{"git push --push-option=ci.skip -f --atomic origin HEAD:dev", "Waiting for you to approve the push to dev"},
		{"git -C /w -C sub push -n origin HEAD:dev", "Waiting for you to approve the push to dev"},
		{"git push origin refs/heads/*:refs/heads/*", "Waiting for you to approve a push"},
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
		{"git push --del origin dev", "deployer asks to run: git push --del origin dev"},
		{"git push -fd origin dev", "deployer asks to run: git push -fd origin dev"},
		{"git push -fu origin dev", "deployer asks to run: git push -fu origin dev"},
		{"git -c core.sshCommand=evil push origin HEAD:dev", "deployer asks to run: git -c core.sshCommand=evil push origin HEAD:dev"},
		{"git --git-dir=/x push origin HEAD:dev", "deployer asks to run: git --git-dir=/x push origin HEAD:dev"},
		{"git push --receive-pack=evil origin HEAD:dev", "deployer asks to run: git push --receive-pack=evil origin HEAD:dev"},
		{"git push --repo=other HEAD:dev", "deployer asks to run: git push --repo=other HEAD:dev"},
		{"git push https://other.example/r HEAD:dev", "deployer asks to run: git push https://other.example/r HEAD:dev"},
		{"git push ../other HEAD:dev", "deployer asks to run: git push ../other HEAD:dev"},
		{"git push origin 'HEAD:dev'", "deployer asks to run: git push origin 'HEAD:dev'"},
		{`git push origin "HEAD:dev"`, `deployer asks to run: git push origin "HEAD:dev"`},
		{"git push origin HEAD:dev -o", "deployer asks to run: git push origin HEAD:dev -o"},
		{"git -C", "deployer asks to run: git -C"},
		{"git -c", "deployer asks to run: git -c"},
		{"git status", "deployer asks to run: git status"},
		{"npm publish", "deployer asks to run: npm publish"},
		{"", "deployer asks to run: "},
	} {
		if got := WaitingLine("deployer", c.command); got != c.want {
			t.Errorf("%q: got %q, want %q", c.command, got, c.want)
		}
	}
}
