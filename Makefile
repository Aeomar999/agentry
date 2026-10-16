# `make` builds the UI, then the binary that embeds it.
# Node is needed only for `make ui`; the built UI is committed, so
# `go build` and `go install` work without it.

.PHONY: all ui build test

all: ui build

ui:
	cd ui && npm ci && npm run build

build:
	go build -o bin/agentry ./cmd/agentry

test:
	go vet ./... && go test ./...
	cd ui && npm run lint
