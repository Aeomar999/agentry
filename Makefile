# `make` builds the UI, then the binary that embeds it.
# Node is needed only for `make ui`; the built UI is committed, so
# `go build` and `go install` work without it.

.PHONY: all ui build workers test

all: ui build workers

ui:
	cd ui && npm ci && npm run build

build:
	go build -o bin/agentry ./cmd/agentry

# The builds a machine joining as a worker downloads, next to bin/agentry,
# for the kinds of machine the server isn't.
WORKER_PLATFORMS = darwin/arm64 darwin/amd64 linux/arm64 linux/amd64

workers:
	for p in $(WORKER_PLATFORMS); do \
		GOOS=$${p%/*} GOARCH=$${p#*/} CGO_ENABLED=0 go build -o bin/agentry-$${p%/*}-$${p#*/} ./cmd/agentry || exit 1; \
	done

test:
	go vet ./... && go test ./...
	cd ui && npm run lint
