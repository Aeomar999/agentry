# Agentry in one image: the server, with its UI built in, and what agents
# need to work: git, tmux and Claude Code. Everything worth keeping lives in
# the agentry user's home, which the stack mounts as a volume (deploy/).

FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/agentry ./cmd/agentry

FROM node:24-bookworm-slim
# Claude Code is pinned, and installed outside the home volume, so each image
# brings its own version; it doesn't update itself.
ARG CLAUDE_CODE_VERSION=2.1.301
ENV DISABLE_AUTOUPDATER=1
RUN apt-get update \
 && apt-get install -y --no-install-recommends git tmux ca-certificates curl openssh-client procps less \
 && rm -rf /var/lib/apt/lists/* \
 && npm install -g "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}" \
 && npm cache clean --force \
 && useradd --create-home --uid 1001 --shell /bin/bash agentry
COPY --from=build /out/agentry /usr/local/bin/agentry
COPY deploy/entrypoint.sh /usr/local/bin/agentry-entrypoint
USER agentry
WORKDIR /home/agentry
EXPOSE 7800
ENTRYPOINT ["agentry-entrypoint"]
