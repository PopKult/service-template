# service-template

Template for every PopKult Go microservice, implementing
[`docs/microservice-standards.md`](docs/microservice-standards.md) — the
binding spec for how these services are built. Read that doc first; this
README covers what's specific to using the template itself.

This is a **bare skeleton**: real, working infrastructure (config
loading, dependency wiring, health checks, graceful shutdown, the
transactional outbox, Dockerfile, k8s manifests, CI) with **zero business
logic**. It compiles, lints, and passes its tests as-is — there's no
invented example domain to strip out.

## Turning this into a new service

Because internal package names are already generic (`registry`,
`config`, `postgres`, `grpcserver`, ...) and the service's own name is
read from the `SERVICE_NAME` env var at runtime rather than hardcoded in
Go source, renaming is a small, mechanical surface — a literal string
replace of `service-template` → `<new-service>` (kebab-case) in exactly
these places:

1. `go.mod` — module path
2. `deployments/docker-compose.yml` — service names, image names, DB/user names
3. `deployments/k8s/*.yaml` — resource names, labels, `ConfigMap`/`Secret` refs
4. `.github/workflows/ci.yml` — image tag
5. This README's title

No Go template syntax, no package renames. Steps for a Claude skill (or a
human) generating `order-service` from this template:

```
cp -r service-template order-service
cd order-service
grep -rl 'service-template' . --exclude-dir=.git | xargs sed -i '' 's/service-template/order-service/g'
sed -i '' 's#module github.com/PopKult/order-service#module github.com/PopKult/order-service#' go.mod  # already correct after the sed above
go mod tidy
git init && git add -A && git commit -m "Initial order-service from service-template"
```

Then, per `docs/microservice-standards.md`:

- **Stop and ask** which GraphQL gateway topology applies (§2.3) before
  touching `internal/entrypoint/graphql/` — this template deliberately
  leaves it unwired.
- Add the service's first domain type in `internal/domain/`, first use
  case in `internal/usecase/<usecase>/`, per the conventions documented
  in each package's `doc.go`.
- Set a real `OUTBOX_TOPIC` and register the service's own Kafka
  consumer in `internal/entrypoint/kafkaconsumer/` if it needs one.

## What's actually implemented

| Piece | Where |
|---|---|
| Typed config, fail-fast | `internal/config` |
| DI wiring (`cmd/server`'s graph) | `internal/registry/registry.go` |
| DI wiring (`cmd/outbox-relay`'s graph) | `internal/registry/relay_registry.go` |
| Postgres pool | `internal/repository/postgres` |
| Transactional outbox: write + relay | `internal/repository/outbox` |
| gRPC server: RED metrics, panic recovery, tracing, health | `internal/entrypoint/grpcserver` |
| Prometheus `/metrics` endpoint | `internal/entrypoint/metricsserver` |
| Outbox table migration | `migrations/0001_create_outbox.{up,down}.sql` |
| Multi-stage Alpine Dockerfile, non-root | `deployments/docker/Dockerfile` |
| Local dev stack (Postgres, Kafka, OTel Collector, both binaries) | `deployments/docker-compose.yml` |
| k8s Deployment/Service/ConfigMap + migration Job | `deployments/k8s/` |
| CI: lint, test (incl. integration), govulncheck, Trivy, gitleaks, build | `.github/workflows/ci.yml` |

Library choices the standards doc doesn't pin (its Quick Reference table
has no entry for these) — picked here, swap freely if the team prefers
otherwise:

- **Postgres driver:** [`jackc/pgx/v5`](https://github.com/jackc/pgx) (`pgxpool`)
- **Kafka client:** [`segmentio/kafka-go`](https://github.com/segmentio/kafka-go) (pure Go, no cgo)

## Local development

```
cd deployments && docker compose up --build
```

Or run outside Docker against `docker compose up postgres kafka otel-collector`:

```
cp .env.example .env
set -a && source .env && set +a
make run          # cmd/server
make run-relay    # cmd/outbox-relay, separately
```

```
make test              # unit tests
make test-integration  # + testcontainers-go integration tests (needs Docker)
make lint
```

## CI secrets required

- `POPKULT_MODULES_TOKEN` — a GitHub token with read access to
  `github.com/PopKult/go-common` and `github.com/PopKult/schema`, used to
  authenticate `go mod download` for these private modules both in the
  Go toolchain steps and inside the Docker build (via a build secret —
  see `deployments/docker/Dockerfile` and `.github/workflows/ci.yml`).

## Private module auth (local dev)

```
go env -w GOPRIVATE=github.com/PopKult/*
```

Plus normal git auth (SSH key or HTTPS token) for `github.com/PopKult/*`
— no separate module proxy infrastructure, per
`docs/microservice-standards.md` §9.
