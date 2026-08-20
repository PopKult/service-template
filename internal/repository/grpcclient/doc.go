// Package grpcclient holds outbound gRPC clients to other services, one
// subpackage per service called, each wrapped behind a use-case-shaped
// interface defined where it's consumed (accept interfaces, return
// structs) per docs/microservice-standards.md §1.2. Wire
// go-common/middleware/grpcmw.TracingClientStatsHandler() into every
// client connection so outbound calls stay part of the same trace.
//
// Nothing lives here yet — this is a template. Add a client subpackage
// the first time a use case needs to call another service.
package grpcclient
