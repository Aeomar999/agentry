# `make` builds the UI, then the binary that embeds it.
# Node is needed only for `make ui`; the built UI is committed, so
# `go build` and `go install` work without it.

.PHONY: all ui build workers test spec package desktop release-check

all: ui build workers

ui:
	cd ui && npm ci && npm run build

build:
	go build -o bin/agentry ./cmd/agentry

# The builds a machine downloads, next to bin/agentry, for the kinds of
# machine the server isn't: joining as a worker, or connecting Claude Code.
WORKER_PLATFORMS = darwin/arm64 darwin/amd64 linux/arm64 linux/amd64 windows/amd64 windows/arm64

workers:
	for p in $(WORKER_PLATFORMS); do \
		GOOS=$${p%/*} GOARCH=$${p#*/} CGO_ENABLED=0 go build -o bin/agentry-$${p%/*}-$${p#*/} ./cmd/agentry || exit 1; \
	done

test:
	spec/check
	go vet ./... && go test ./...
	cd cmd/agentry-desktop && CGO_ENABLED=0 go vet . && CGO_ENABLED=0 go test .
	cd ui && npm run lint

# The spec folder's checks: every scenario tagged, numbered and judged.
spec:
	spec/check

# The downloads: a zip for each kind of computer, in release/, with all it
# needs to run Agentry (Start Agentry opens it in the browser).
package: ui
	go run ./packaging

# The desktop app with the server it starts, in release/: on Linux, a
# .tar.gz for Linux and a zip for Windows; on a Mac, the Mac app's zip
# (see README, Make a release). It uses the committed page, so it needs no
# Node.
desktop:
	go run ./packaging desktop

# Before publishing: checks release/ holds the tag's 9 files, built from the
# tag's commit with no changes, writes SHA256SUMS and prints the gh command.
release-check:
	go run ./packaging check $(VERSION)
