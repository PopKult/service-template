// Package repository holds one subpackage per backing store or outbound
// call a use case depends on: postgres/ for this service's own tables,
// grpcclient/ for outbound calls to other services, outbox/ for the
// transactional outbox (§1.3). The interface a use case depends on is
// small, use-case-shaped (not generic CRUD), and defined where it's
// consumed, per standard Go convention (accept interfaces, return
// structs). See docs/microservice-standards.md §1.2 in the repo root.
package repository
