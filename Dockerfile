# syntax=docker/dockerfile:1

# ---- typst: fetch the static musl binary (no cgo, no libc needed at runtime) ----
FROM debian:bookworm-slim AS typst
ARG TYPST_VERSION=0.15.1
RUN apt-get update && apt-get install -y --no-install-recommends curl xz-utils ca-certificates \
    && curl -fsSL -o /tmp/typst.tar.xz \
       "https://github.com/typst/typst/releases/download/v${TYPST_VERSION}/typst-x86_64-unknown-linux-musl.tar.xz" \
    && tar -xJf /tmp/typst.tar.xz -C /tmp \
    && mv "/tmp/typst-x86_64-unknown-linux-musl/typst" /usr/local/bin/typst \
    && chmod +x /usr/local/bin/typst

# ---- build: compile the static Go binary. Static assets and migrations are embedded
# via go:embed, and the templ-generated *_templ.go files are committed, so this stage
# needs no extra tooling beyond the Go toolchain. ----
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG BUILD_SHA=dev
ARG BUILD_TIME=
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w -X main.buildSHA=${BUILD_SHA} -X main.buildTime=${BUILD_TIME}" -o /out/cladex ./cmd/server

# ---- final: distroless, no shell. Exec the binary directly for cladexctl-style admin
# commands (docker compose exec cladex /cladex ...), never `exec ... sh`. Not the
# :nonroot tag — it locks in uid 65532, which can't own the NAS's PUID-1000 bind mount.
# Compose sets `user:` at deploy time instead, matching the mount's owner. ----
FROM gcr.io/distroless/static-debian12 AS final
ENV PATH="/usr/local/bin:/bin:/usr/bin"
COPY --from=typst /usr/local/bin/typst /usr/local/bin/typst
COPY --from=build /out/cladex /cladex
EXPOSE 8090
ENTRYPOINT ["/cladex"]
