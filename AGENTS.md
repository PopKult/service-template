# service-template

This repository is a scaffold, not a deployed business service. Preserve its
zero-domain starting point. New service creation must choose its GraphQL topology
and replace this template-specific guidance. Keep Dockerfile migrations, outbox,
health and telemetry wiring consistent with prod-setup's template manifests.

Read [README.md](README.md), [docs/microservice-standards.md](docs/microservice-standards.md) before related work.

Follow this service's own standards and documented exceptions. Keep entrypoints
thin, one Execute operation per use case, repository interfaces near their consumers,
and constructor DI in internal/registry. Publish Kafka events through the transactional
outbox; keep consumers idempotent and propagate trace context. Avoid logging tokens,
practice text or other sensitive payloads.

Contracts live in the sibling schema repository; use additive protobuf/GraphQL
changes and real semver dependencies. Runtime manifests live in prod-setup and
local compose topology in local-setup. Verify sibling origins before editing them.
Migrations ship in the service image and must work with the previous release;
production abort reverts code, never the database schema. Relevant documentation
may lag code: surface concrete conflicts before changing established behavior.

Use a feature branch, including for scaffolding and documentation. Preserve the
user's local work; use an isolated checkout when needed. Complete the agreed task
without per-commit approval pauses. Ask about material missing decisions, reusing
answers already given. Push/open PRs when requested; do not merge them.

Relevant shared skills: `init-service`.
The canonical bundle is [https://github.com/PopKult/claude-skills](https://github.com/PopKult/claude-skills); install with its
scripts/install-skills.py into ~/.agents/skills for local Codex/Cursor.
A sibling checkout alone does not make skills discoverable.

## Verification

For Go changes run go build ./..., go vet ./..., go test ./... and
golangci-lint run ./.... For storage/SQL/relay changes also run
go test -tags=integration ./... with Docker. Inspect Makefile and CI for any
additional checks relevant to the change. Private modules require the existing
PopKult authentication setup; never put credentials into source files.

For documentation-only changes, check accuracy and links instead of rebuilding
the application. Report checks run and any unavailable prerequisites.
