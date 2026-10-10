# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build
WORKDIR /src

# GOPRIVATE + git auth for github.com/PopKult/* modules, per
# docs/microservice-standards.md §9. CI supplies GONOSUMCHECK-safe
# credentials via a build secret; see .github/workflows/ci.yml.
ENV GOPRIVATE=github.com/PopKult/*
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN --mount=type=secret,id=netrc,target=/root/.netrc \
    go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server
RUN CGO_ENABLED=0 go build -o /out/outbox-relay ./cmd/outbox-relay

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && \
    addgroup -S app && adduser -S app -G app
USER app

COPY --from=build /out/server /usr/local/bin/server
COPY --from=build /out/outbox-relay /usr/local/bin/outbox-relay

ENTRYPOINT ["/usr/local/bin/server"]
