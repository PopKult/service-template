# PopKult Go Microservice Standards

## 0. Purpose, scope & how to read this

This document defines how every PopKult Go microservice is built, so that
any engineer — human or AI agent — can create or modify a service without
re-deriving conventions. It is written to be **executable by an AI agent
scaffolding or reviewing a service**, not just read by a human, so:

- Requirement language follows [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119):
  **MUST** / **MUST NOT** = no exceptions without amending this doc.
  **SHOULD** / **SHOULD NOT** = do this unless there's a documented reason
  not to. **MAY** = optional, include only if the service needs it.
- Every technology choice below is a single canonical option, not a menu.
  Where a real alternative exists it's named in a short rationale, but the
  default is what an agent should generate — do not pick a different
  library or pattern than the one named here without the user explicitly
  asking for a deviation.
- Placeholders: `<service>` = kebab-case service name (e.g. `order-service`),
  `<domain>` = singular lowercase business domain (e.g. `order`), `<n>` =
  integer proto version (`v1`, `v2`, ...), `<usecase>` = lowercase use-case
  name (e.g. `createorder`).
- A few points are genuinely undecided rather than merely unwritten. Each
  is marked **⚠️ AGENT: CONFIRM WITH USER** — an agent must not silently
  pick an answer there; stop and ask. See also [Deferred decisions](#deferred-decisions).
- When this doc and a service's existing code disagree, this doc wins —
  the code is the thing that's wrong.

**Repo model:** one GitHub repo per service, `github.com/PopKult/<service>`,
holding that service's code and container image build only — no k8s
manifests, no docker-compose. Four additional repos are shared
infrastructure for every service:

| Repo | Contents |
|---|---|
| `github.com/PopKult/schema` | all `.proto` and GraphQL schema files, buf-managed |
| `github.com/PopKult/go-common` | shared Go library: logging, config, middleware |
| `github.com/PopKult/prod-setup` | k8s manifests for every service, one directory per service |
| `github.com/PopKult/local-setup` | local dev docker-compose stack, shared infra + one block per service |

All four are private; `schema` and `go-common` are versioned with semver
tags (see [§9](#9-shared-code--infra-repos)), `prod-setup` and
`local-setup` are plain manifests/compose tracked at `main` (nothing
`go get`s them, so no tagging discipline is needed).

---

## Quick reference

| Concern | Decision |
|---|---|
| Directory layout | [golang-standards/project-layout](https://github.com/golang-standards/project-layout) + clean-architecture split (§1) |
| Business logic location | `internal/usecase/<usecase>`, one `Execute(ctx, in) (out, error)` method each |
| Dependency wiring | `internal/registry`, manual constructor-based DI, no DI framework |
| Sync inter-service calls | gRPC, schema from `github.com/PopKult/schema` |
| Async inter-service events | Kafka, Protobuf, via outbox pattern only |
| Client-facing API | GraphQL |
| Error handling | wrap with `%w` + sentinel/typed errors in `internal/domain`, propagate through all layers |
| Logging | `log/slog`, JSON to stdout only |
| Unit test framework | stdlib `testing`, table-driven, no assertion library |
| Mocking | `go.uber.org/mock` (`uber-go/mock`), generated via `go:generate` |
| Integration tests | `testcontainers-go`, scoped to `internal/repository` + outbox relay only |
| Tracing | OpenTelemetry SDK → OTel Collector → Jaeger |
| Metrics | Prometheus client, RED method, auto-instrumented via `go-common` middleware |
| Log shipping | stdout → Filebeat DaemonSet → Logstash → Elasticsearch → Kibana |
| Config loading | `github.com/caarlos0/env/v9` into a typed `Config` struct, fail-fast at startup |
| Secrets (prod) | k8s `Secret` sealed with Bitnami Sealed Secrets, committed encrypted |
| Prod deploy target | Kubernetes, manifests in `github.com/PopKult/prod-setup` |
| Local dev | docker-compose, stack in `github.com/PopKult/local-setup` |
| Container base image | multi-stage build → `alpine`, non-root user |
| DB migrations | `golang-migrate`, run as a k8s Job before rollout, never by the app itself |
| Service-to-service auth | mTLS via service mesh (Istio/Linkerd), no app code |
| External client auth | JWT validated at the GraphQL/gRPC edge |
| CI/CD | GitHub Actions, shared reusable workflow |
| Private module auth | `GOPRIVATE=github.com/PopKult/*` + token/SSH git auth |
| Repo naming | kebab-case: `order-service` |
| Proto package naming | `popkult.<domain>.v<n>`: `popkult.order.v1` |
| Password hashing | Argon2id (`golang.org/x/crypto/argon2`), PHC string format |
| PII at rest | Field-level AES-256-GCM before writing to Postgres |
| Log/error redaction | `go-common/secure.String` — `slog.LogValuer`-based, explicit `.Reveal()` |
| Vulnerability scanning | `govulncheck` + Trivy (Critical/High blocks merge) + `gitleaks` (any finding blocks) |

---

## 1. Project structure

Every service MUST use [golang-standards/project-layout](https://github.com/golang-standards/project-layout)
as the outer shell, with a clean-architecture split inside it:
**entrypoints** translate transport into calls on **use cases**, use cases
hold business logic and call **repositories** for data access. Dependencies
point inward — entrypoints depend on use cases, use cases depend on
repository *interfaces* — and MUST NOT point outward.

```
<service>/
├── cmd/
│   ├── server/              # main entrypoint binary: loads config, builds registry, starts servers
│   └── outbox-relay/        # separate binary: publishes outbox rows to Kafka
├── internal/
│   ├── domain/               # entities, value objects, sentinel/typed domain errors
│   ├── usecase/               # one package per business operation
│   │   ├── <usecase>/
│   │   │   └── usecase.go     # type UseCase struct{...}; func (uc *UseCase) Execute(ctx, in) (out, error)
│   │   └── ...
│   ├── repository/            # interfaces + implementations, one subpackage per backing store
│   │   ├── postgres/
│   │   ├── grpcclient/        # outbound calls to other services
│   │   └── outbox/            # transactional outbox writer + relay reader
│   ├── entrypoint/
│   │   ├── grpcserver/
│   │   ├── kafkaconsumer/
│   │   ├── graphql/
│   │   └── metricsserver/     # Prometheus /metrics HTTP endpoint
│   ├── audit/                 # OPTIONAL — only if the service has admin actions, see §1.4
│   ├── registry/               # wires repositories → use cases → entrypoints, see §1.5
│   └── config/                 # typed Config struct + loader, see §6
├── migrations/                 # golang-migrate SQL files
├── deployments/
│   └── docker/Dockerfile
├── go.mod
└── README.md
```

A service repo holds its own code and container image build only. It
MUST NOT contain k8s manifests or a docker-compose file — those live in
`github.com/PopKult/prod-setup` and `github.com/PopKult/local-setup`
respectively (see [§7](#7-deployment--runtime)), one directory/block per
service in each, so cluster and local-stack topology changes don't
require a code review in the service repo.

Generated gRPC/GraphQL code MUST NOT be vendored locally — it's imported as
a versioned Go module published from `github.com/PopKult/schema` (see
[§9](#9-shared-code--infra-repos)). A service repo MUST NOT contain a local
`api/` directory of `.proto` files.

### 1.1 Use cases

- One use case = one distinct business operation. Name it so the name
  alone explains the logic: `CreateOrderUseCase`, `CancelOrderUseCase` —
  MUST NOT be a grab-bag type like `OrderUseCase` with multiple methods.
- Every use case MUST expose exactly one method: `Execute(ctx, input) (output, error)`.
  Needing a second method is a signal the type should split into two use
  cases.
- Use cases SHOULD be as small and single-purpose as possible. Use cases
  MAY call other use cases to compose behavior — e.g. `PlaceOrderUseCase`
  calling `ReserveInventoryUseCase.Execute()` — instead of duplicating
  logic or reaching into another use case's repositories directly.
- Use cases MUST depend on repository *interfaces* defined next to them
  (or in `domain`), never on concrete repository types — this is what
  makes them unit-testable with generated mocks (§4).
- Entrypoints and repositories MUST NOT contain business logic.
  Entrypoints translate transport ↔ use case input/output; repositories
  translate domain calls ↔ storage/network calls. A rule about *when*
  something is allowed to happen belongs in `internal/usecase` — finding
  one anywhere else is a bug to fix, not a style nit.

### 1.2 Repositories

A repository is anything that gets or persists data on behalf of a use
case: a Postgres table, a gRPC call to another service, a Kafka publish.
Each gets its own subpackage under `internal/repository/`. The interface a
use case depends on MUST be small and use-case-shaped (not a generic CRUD
interface) and MUST be defined where it's consumed, per standard Go
convention (accept interfaces, return structs).

### 1.3 Outbox pattern for Kafka

Use cases MUST NOT publish to Kafka directly. Instead:

1. The use case writes its state change **and** an outbox row in the same
   Postgres transaction, via `internal/repository/outbox`.
2. `cmd/outbox-relay` — a separate process — polls unpublished outbox
   rows, publishes them to Kafka as Protobuf messages, and marks them
   published.

This guarantees a Kafka event is only sent if the DB transaction it
describes actually committed, and gives at-least-once delivery. Consumers
MUST be idempotent — the outbox relay's polling can redeliver.

### 1.4 Audit package (optional)

Services with admin-triggered mutations MAY include `internal/audit`: an
event-sourced, append-only log of who did what, when, and the before/after
state. Only add it if the service actually has admin actions worth
auditing — it is not boilerplate every service needs. Audit-triggering use
cases write an audit event as part of the same flow that performs the
mutation, typically via the outbox on a dedicated audit topic, so audit
history is queryable independent of the service's own DB.

### 1.5 Dependency wiring (registry)

Every service MUST construct its dependency graph in one place:
`internal/registry`. This is plain, explicit, manual dependency injection —
no reflection-based DI framework (`google/wire`, `uber/fx`) — construction
order is just Go code, so a reader (human or agent) can trace exactly what
gets built and in what order without learning a separate tool.

```go
// internal/registry/registry.go
type Registry struct {
    // entrypoints, exported so cmd/server can start them
    GRPCServer     *grpcserver.Server
    KafkaConsumer  *kafkaconsumer.Consumer
    GraphQLHandler *graphql.Handler
}

func New(ctx context.Context, cfg config.Config) (*Registry, error) {
    // 1. infra clients: DB pool, Kafka client, outbound gRPC clients
    db, err := postgres.NewPool(ctx, cfg.Postgres)
    if err != nil {
        return nil, fmt.Errorf("registry: connect postgres: %w", err)
    }

    // 2. repositories, built from infra clients
    orderRepo := postgres.NewOrderRepository(db)
    outboxRepo := outbox.NewRepository(db)

    // 3. use cases, built from repositories (and other use cases)
    createOrder := createorder.New(orderRepo, outboxRepo)

    // 4. entrypoints, built from use cases
    grpcServer := grpcserver.New(createOrder)

    return &Registry{GRPCServer: grpcServer}, nil
}
```

Rules:

- Construction MUST follow the dependency direction from §1: infra
  clients → repositories → use cases → entrypoints. `New` MUST return an
  error (not panic) on any failed construction step (e.g. a DB connection
  that can't be established), wrapped with enough context to identify
  which dependency failed.
- `Registry` MUST only hold what `cmd/server/main.go` needs to start and
  stop the service — entrypoints (to serve) and anything needing explicit
  cleanup (e.g. the DB pool, to close on shutdown). It MUST NOT be used as
  a general-purpose service locator passed deep into use cases — use cases
  take their specific dependencies as constructor arguments, not the
  `Registry` itself.
- `cmd/outbox-relay` builds its own smaller registry (infra clients →
  outbox repository → relay), independent of `cmd/server`'s — the two
  binaries don't share a `Registry` instance, only the `go-common` and
  repository packages.
- If wiring in `internal/registry` grows unwieldy (rare for a
  single-purpose microservice), consider `google/wire` to generate the
  same explicit code from constructor functions — this is a mechanical
  optimization, not a change in pattern, and MUST still produce plain
  constructor calls, not a runtime container.

---

## 2. Communication between services

| Interaction | Mechanism |
|---|---|
| Sync, service-to-service | gRPC |
| Async, service-to-service | Kafka (via outbox, §1.3) |
| Client-facing | GraphQL |

### 2.1 gRPC

- All `.proto` files live in `github.com/PopKult/schema`, managed with
  [buf](https://buf.build). `buf lint` and `buf breaking` run in that
  repo's CI and MUST block merges that break compatibility.
- Generated Go stubs are published from the schema repo as a versioned Go
  module; services import it like any other dependency — never generate
  or check in stubs locally.
- Proto packages MUST be dot-namespaced: `popkult.<domain>.v<n>` (e.g.
  `popkult.order.v1`).
- Changes MUST be additive-only: add fields/RPCs, never remove or rename
  live ones. A genuinely breaking change gets a new versioned package
  (`v2`) served alongside `v1` during migration — never an in-place edit
  of a released message/RPC.
- `internal/entrypoint/grpcserver` MUST stay thin: unmarshal request →
  call exactly one use case → map result/error to response (§3.3).

### 2.2 Kafka

- Message format MUST be Protobuf, using the same schema-repo definitions
  and tooling as gRPC, validated against a schema registry for
  compatibility.
- Messages MUST only be published via the outbox pattern (§1.3) — no
  direct produce calls from a use case or entrypoint.
- Trace context MUST propagate through Kafka: the outbox relay injects
  the OpenTelemetry trace context into message headers on publish, and
  `internal/entrypoint/kafkaconsumer` extracts it, so an async flow shows
  up as one continuous trace rather than disconnected fragments (§5.2).
  The relay polls in its own loop, so it has no request context: the
  outbox row stores the writing request's W3C `traceparent` in a nullable
  `traceparent` column (`outbox.Write` calls `kafkamw.Capture`), and the
  relay calls `kafkamw.Restore` per row before injecting the headers.

### 2.3 GraphQL

GraphQL is the client-facing layer. Two things are fixed regardless of
topology:

- The GraphQL layer MUST NOT contain business logic — resolvers call use
  cases (directly if colocated, or via gRPC to the owning service) the
  same way any other entrypoint does.
- Schema files live in `github.com/PopKult/schema`, under the same
  additive-only, buf-gated discipline as `.proto` files.
- External clients authenticate via JWT, validated at this edge (§8.2);
  calls onward carry forwarded identity via context/metadata, not raw
  end-user credentials.

**⚠️ AGENT: CONFIRM WITH USER** — gateway topology (a single BFF service
that calls other services over gRPC, vs. federated subgraphs composed by
a federation gateway) is genuinely undecided and depends on which clients
the service serves. Do not choose one silently when scaffolding a new
service's GraphQL entrypoint — ask which topology applies.

---

## 3. Error handling & logging

### 3.1 Error handling

- Wrap with `fmt.Errorf("...: %w", err)` for context; define sentinel or
  typed errors in `internal/domain` (`var ErrNotFound = errors.New(...)`
  or a typed `NotFoundError{...}`); check with `errors.Is` / `errors.As`.
  This is the only error-handling idiom in use — no error-handling
  libraries beyond the stdlib.
- Errors MUST propagate through every layer — repository → use case →
  entrypoint — each layer adding context via wrapping. An error MUST NOT
  be silently swallowed or logged-and-dropped partway up; the entrypoint
  is what finally turns it into a response to the caller (gRPC status,
  GraphQL error, or a NACK/retry decision for a Kafka consumer).
- Panics in any entrypoint (gRPC interceptor, Kafka consumer wrapper,
  GraphQL resolver middleware) MUST be caught by the recover middleware
  provided by `go-common`: log the stack trace, convert to an Internal
  error, keep the process alive. A single bad request MUST NOT take down
  a pod.

### 3.2 Logging

- `log/slog` (stdlib) only, structured JSON to stdout. No third-party
  logging library.
- Every log line MUST carry: timestamp, level, service name, `trace_id`,
  `span_id`, and the use case / layer that emitted it, plus whatever
  structured attributes are relevant to that line.
- Log levels, strictly:
  - **Error** — needs human attention; something is broken.
  - **Warn** — degraded but handled (e.g. a retry succeeded, a fallback
    was used).
  - **Info** — key business events only (use case started/completed,
    "order created") — MUST NOT be used for every function call.
  - **Debug** — verbose, dev-only detail; off by default in prod, toggled
    via env var.
- Logs ship via a node-level shipper (Filebeat/Fluent Bit DaemonSet in
  k8s) to Elastic. A service MUST NOT hold an Elastic client or talk to
  Elastic directly (§5.3).

### 3.3 Mapping errors to transport

Each entrypoint MUST maintain an explicit mapping table from domain
errors to its transport's error format:

- gRPC: sentinel/typed domain error → canonical `google.golang.org/grpc/codes` code.
- GraphQL: domain error → error `extensions.code`.

Any domain error not in the table MUST map to `Internal` / `INTERNAL_ERROR`
by default — new domain errors must never silently leak as a 200 or a
vague message; mapping gaps fail closed, not open.

---

## 4. Testing

- **Framework:** stdlib `testing`, table-driven tests. No assertion
  library (no testify) — plain `if`/`t.Errorf`.
- **Mocking:** repository and use-case interfaces get generated mocks via
  `go.uber.org/mock` (`uber-go/mock`), driven by `go:generate` — never
  hand-maintained.
- **Integration tests:** scoped to `internal/repository` implementations
  and the outbox relay only — the parts unit tests with mocks can't
  verify (real SQL, real Kafka publishing). Use `testcontainers-go` to
  spin up real Postgres/Kafka per test run; no shared test environment.
  Gate these behind a build tag (`//go:build integration`) so
  `go test ./...` stays fast by default. Use cases themselves MUST NOT
  need integration tests — mocked unit tests are sufficient for
  business-logic coverage.
- **Coverage:** reported for visibility (e.g. a PR comment). No hard CI
  gate — a coverage percentage MUST NOT substitute for judgment about
  what's actually worth testing.

---

## 5. Observability

Three pillars, three tools, tied together by trace/span IDs:

| Signal | Tool |
|---|---|
| Traces | OpenTelemetry SDK → OTel Collector → Jaeger |
| Metrics | Prometheus client → Prometheus → Grafana (alerts: Alertmanager → Telegram) |
| Logs | `log/slog` → stdout → Filebeat → Logstash → Elasticsearch → Kibana |

All telemetry export MUST go through an OTel Collector rather than each
service talking to each backend directly: services export OTLP to the
collector, which forwards traces to Jaeger. Metrics are the exception:
Prometheus scrapes each service's `/metrics` port directly. This keeps
app-side instrumentation vendor-neutral.

Every binary (`cmd/server` and `cmd/outbox-relay`) MUST call
`go-common/telemetry.Init` at startup and flush it on shutdown. Without
it the global tracer provider and W3C propagator are no-ops: no spans are
created and no trace context crosses a gRPC or Kafka hop, even though the
middleware looks wired in.

**What to log.** Log business decisions and failures, not plumbing: one
`Info` line when a use case completes a state change that matters (with
the IDs involved, never the payload), one `Warn` for a rejected or
degraded-but-handled case, and `Error` exactly once where an error is
finally handled (§3.1). Always use the `*Context` slog variants so
`trace_id`/`span_id` attach. Never log tokens, passwords, emails,
phone numbers or request/response bodies; `go-common/logging` and
Logstash redact common keys as a safety net, but the first line of
defence is not passing them (use `secure.String`).

### 5.1 Metrics

RED method (Rate, Errors, Duration) MUST be auto-instrumented by
`go-common` middleware — zero manual work per service — on:

- Every gRPC method (server-side interceptor)
- Every Kafka consumer/producer
- Every GraphQL resolver

Services MAY additionally define business-level metrics
(`orders_processed_total`, etc.) on top of RED.

### 5.2 Tracing

- OpenTelemetry SDK, context propagated automatically through gRPC calls
  (interceptors from `go-common`) and through Kafka message headers
  (§2.2), so one business flow spanning sync + async hops shows up as a
  single trace.
- Every log line MUST include the active `trace_id`/`span_id` so a trace
  in the tracing UI pivots directly to matching logs in Kibana.

### 5.3 Logs

Services write structured JSON to stdout only (§3.2). A node-level
Filebeat/Fluent Bit DaemonSet ships those to Elasticsearch; Kibana is the
query UI. No service holds an Elastic client or credential.

### 5.4 Health checks

- **Readiness:** implement the standard [gRPC Health Checking Protocol](https://github.com/grpc/grpc/blob/master/doc/health-checking.md)
  (`grpc.health.v1`), reporting `NOT_SERVING` if a critical dependency
  (DB, Kafka) is unreachable. The k8s readiness probe MUST hit this.
- **Liveness:** a separate, minimal check confirming only that the
  process is alive and not deadlocked. It MUST NOT check downstream
  dependencies — a transient DB blip should take the pod out of rotation
  via readiness, not kill it via liveness.

---

## 6. Configuration & secrets

- Env vars unmarshal into a typed `Config` struct via `github.com/caarlos0/env/v9`,
  validated once at startup. Fail-fast: a missing required var or an
  invalid value MUST crash the process immediately with a clear error,
  never surface as a mysterious runtime failure later.
- Optional vars MUST have explicit defaults in the struct tags.
- The app's config-loading code MUST be identical in every environment —
  it only ever reads env vars. What differs is how those env vars get
  set:
  - **Local (docker-compose, in `github.com/PopKult/local-setup`):** plain
    `environment:` blocks with dummy dev credentials.
  - **Prod (k8s, in `github.com/PopKult/prod-setup`):** non-sensitive
    config via `ConfigMap`, secrets via `Secret` objects, both injected
    as env vars into the pod.
- **Secrets in prod** MUST be managed as k8s `Secret` manifests sealed
  with [Bitnami Sealed Secrets](https://github.com/bitnami-labs/sealed-secrets)
  so the encrypted form can be committed to git (in `prod-setup`) and
  only the cluster can decrypt it. Plaintext secrets MUST NOT land in git
  under any circumstance. (Migrating to an external secrets manager —
  Vault or a cloud secrets service — is a deferred decision, see below,
  for if rotation/audit needs outgrow Sealed Secrets.)

---

## 7. Deployment & runtime

- **Prod:** Kubernetes, manifests in `github.com/PopKult/prod-setup`
  (`services/<service>/`, one directory per service — see that repo's
  README). **Local:** docker-compose, stack in
  `github.com/PopKult/local-setup` (shared infra + one block per
  service). Neither lives in the service's own repo, so cluster/local
  topology changes don't require a code review there and vice versa.
- **Graceful shutdown:** every service MUST use the drain-pattern
  lifecycle helper from `go-common` — on SIGTERM, stop accepting new
  work, let in-flight gRPC/Kafka/GraphQL requests finish within a fixed
  timeout (default 20s), then exit.
- **Dockerfile:** multi-stage build — a build stage compiles the binary,
  the final image is `alpine`, running as a non-root user. Alpine (not
  distroless/scratch) so a shell is available for debugging in-cluster.
  Lives in the service repo (`deployments/docker/Dockerfile`) since
  building the image is the service repo's job.
- **Migrations:** `golang-migrate`, plain up/down SQL files in the
  service repo's `migrations/`, run as a k8s Job defined in `prod-setup`
  (or a CI step) before the new version's pods roll out. The app itself
  MUST NOT auto-run migrations on startup — concurrent replicas would
  race.
- **Migrations MUST be backward compatible with the previous release.**
  Rollouts are canaries (Argo Rollouts, 50% then 100%), so for a while
  the old and the new version run against the same schema, and a rollback
  reverts code but never the schema. Use expand → contract: add columns
  nullable or with a default; rename or retype by adding the new column,
  writing both, switching reads, and only then dropping the old one in a
  later release; add `NOT NULL` only after backfilling. A migration that
  drops or renames something the previous release still uses is a
  defect. `down` files exist for local development, not as a prod
  rollback mechanism.
- **Rollout:** CI pushes the image to Docker Hub; a commit bumping the tag
  in `prod-setup` is deployed by Argo CD as an Argo Rollouts canary (50%,
  manual promote or metric analysis, then 100%).
- **Backups:** PostgreSQL is backed up with pgBackRest (daily full + WAL
  archiving, point-in-time recovery) to object storage outside the
  database machines. A restore MUST be rehearsed, not assumed.
- **Service-to-service auth:** a service mesh (Istio/Linkerd) provides
  transparent mutual TLS and identity between all services via sidecar
  proxies. Application code makes plain gRPC calls to another service's
  k8s DNS name; the mesh transparently upgrades the connection to mTLS.
  Services MUST NOT implement their own internal-call auth.

---

## 8. CI/CD & auth

### 8.1 CI/CD

GitHub Actions, via a shared reusable workflow defined once and called
from every service repo, covering: lint (`golangci-lint`) → unit test →
security scans (`govulncheck`, Trivy, `gitleaks` — §11.4) → build → build &
push Docker image → deploy trigger. `github.com/PopKult/schema` has its own
pipeline running `buf lint`/`buf breaking` and publishing the generated Go
module on merge.

### 8.2 External client auth

Separate from the mesh's internal mTLS: external clients (apps hitting
GraphQL, third parties hitting a public gRPC surface) authenticate with a
JWT issued by the auth service/IdP, validated once at the edge (the
GraphQL gateway or public-facing entrypoint). The caller's identity is
forwarded internally via context/metadata — internal services trust the
mesh identity for service-to-service calls and the forwarded claims for
"on behalf of user X."

---

## 9. Shared code & infra repos

- **`github.com/PopKult/go-common`:** logging setup, config loader,
  gRPC/Kafka/GraphQL middleware (RED metrics, tracing propagation,
  recover-and-map, graceful-shutdown runner), error-mapping helpers.
  Versioned with semver tags; services pin a version in `go.mod` like any
  dependency.
- **`github.com/PopKult/schema`:** all `.proto` and GraphQL schema files,
  buf-managed, publishes generated Go stubs as a versioned module.
- **`github.com/PopKult/prod-setup`:** k8s manifests for every service —
  plain manifests (no Helm, no Kustomize), one directory per service
  (`services/<service>/`), tracked at `main`. See [§7](#7-deployment--runtime).
- **`github.com/PopKult/local-setup`:** the local dev docker-compose
  stack — shared infra (Postgres, Kafka, OTel Collector) plus one
  build+run block per service, tracked at `main`. See
  [§7](#7-deployment--runtime).
- All four are private. `go-common`/`schema` are Go modules — services
  (and CI) authenticate to `go get` them via `GOPRIVATE=github.com/PopKult/*`
  plus token/SSH-based git auth, no separate module proxy infrastructure.
  `prod-setup`/`local-setup` are plain git clones, not Go modules — no
  `GOPRIVATE` needed, just normal git auth to clone a private repo.

---

## 10. Naming conventions

- **Repos:** kebab-case, e.g. `order-service`.
- **Proto packages:** dot-namespaced, `popkult.<domain>.v<n>`, e.g.
  `popkult.order.v1`.

---

## 11. Security

Authentication/authorization is already covered elsewhere — mTLS via
service mesh for internal calls (§7), JWT at the edge for external clients
(§8.2) — this section covers data protection, redaction, and scanning.

### 11.1 Passwords

Passwords MUST be hashed with **Argon2id** (`golang.org/x/crypto/argon2`),
never stored or logged in any reversible form. Store the algorithm
parameters, salt, and hash together as a single encoded string (the
standard PHC string format) so parameters can change over time without
breaking verification of older hashes. Comparison MUST use the decode
step's built-in constant-time comparison — never `==` on raw hash bytes.

### 11.2 PII at rest

PII that must be read back — email, phone, physical address, government
ID, payment details — MUST be encrypted at the field level with
**AES-256-GCM** before being written to Postgres; the DB column holds
ciphertext only. This is in addition to, not instead of, disk/volume
encryption at the infra level — field-level encryption is what still
protects the data if the database itself is compromised or a backup
leaks.

- Encryption/decryption is a **repository-layer** concern: use cases work
  with plaintext domain value objects, the Postgres repository
  encrypts/decrypts on the way in/out — the same place repositories
  already own storage-shape translation.
- Encryption keys MUST NOT be hardcoded or committed. They're delivered
  the same way as other secrets today (k8s `Secret` via Sealed Secrets,
  §6); store a key **version** alongside each ciphertext so keys can
  rotate without breaking decryption of existing data. Moving key
  management to a KMS/Vault transit engine tracks the same deferred
  decision as the broader move off Sealed Secrets (see below).
- Passwords (§11.1) are hashed, never encrypted — there is no legitimate
  reason to read a password back in plaintext.

### 11.3 Redaction in logs and errors

Any sensitive value (PII, tokens, secrets) that a use case or repository
handles MUST be wrapped in a `go-common` type that redacts itself by
construction rather than by convention:

```go
// go-common/secure
type String string

func (s String) String() string        { return "[REDACTED]" }
func (s String) LogValue() slog.Value  { return slog.StringValue("[REDACTED]") }
func (s String) Reveal() string        { return string(s) }
```

Because `String()` and `LogValue()` both redact, passing a `secure.String`
straight into `slog`, `fmt.Errorf`, or an error message is safe by
default — the plaintext is only ever reachable via the explicit
`.Reveal()` call a developer has to write on purpose (e.g. right before
hashing, encrypting, or building a signed JWT claim). Domain types with
sensitive fields (`User.Email`, `User.Phone`, etc.) MUST use `secure.String`
for those fields, not `string`.

### 11.4 Scanning

CI (§8.1) MUST run, per service, on every PR:

- **`govulncheck`** — known vulnerabilities in Go stdlib and dependencies.
- **Trivy** (or equivalent) — OS/package vulnerabilities in the built
  container image.
- **`gitleaks`** (or equivalent) — blocks commits/PRs that introduce
  hardcoded secrets, independent of the Sealed Secrets discipline in §6.

Critical/High findings from `govulncheck` and Trivy MUST block merge;
Medium/Low findings are reported on the PR but MUST NOT block. Any
`gitleaks` finding MUST block merge — there's no acceptable severity
tier for a committed secret.

### 11.5 Input validation

External input MUST be validated before it reaches a use case — entrypoints
own this, the same way they own transport translation and error mapping
(§3.3). Validation constraints MUST be declared on the schema itself
(`buf` [protovalidate](https://github.com/bufbuild/protovalidate) rules
in `.proto`, equivalent constraint directives in the GraphQL schema) so
the constraint travels with the contract in `github.com/PopKult/schema`
rather than being reimplemented ad hoc per service.

### 11.6 Transport

The service mesh (§7) secures hop-to-hop traffic *inside* the cluster.
Anything reachable from outside it — the GraphQL gateway, any public gRPC
surface — MUST additionally terminate TLS at the ingress (`cert-manager` +
k8s `Ingress`/gateway). Mesh mTLS is not a substitute for ingress TLS;
they secure different hops.

---

## Deferred decisions

Not yet standardized. An agent hitting one of these MUST ask the user
rather than invent a convention silently, and a human making the call
should propose an addition to this doc so it doesn't stay a per-service
guess:

- GraphQL gateway topology (single BFF vs. federated subgraphs) — §2.3.
- README/Makefile structure — currently per-service, not templated.
- Grafana dashboard provisioning (golden-signal dashboard per service) —
  worth automating once enough services exist to justify it.
- External secrets manager (Vault/cloud) as a replacement for Sealed
  Secrets — revisit if rotation/audit needs grow.
