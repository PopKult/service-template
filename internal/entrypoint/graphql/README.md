# graphql/

Not wired up in this template.

`docs/microservice-standards.md` §2.3 marks GraphQL gateway topology
(single BFF calling services over gRPC, vs. federated subgraphs composed
by a federation gateway) as **⚠️ AGENT: CONFIRM WITH USER — undecided
per-service**. An agent scaffolding a real service from this template
MUST stop and ask which topology applies rather than picking one
silently.

Once decided, this package's resolvers should stay thin: translate
GraphQL request/response shapes, call exactly one use case (directly if
colocated, or via `internal/repository/grpcclient` otherwise) — no
business logic here, same as any other entrypoint.
