// Package usecase holds one subpackage per business operation. Each
// subpackage exports exactly one type with a single method:
//
//	type CreateWidgetUseCase struct{ /* repository interfaces */ }
//	func (uc *CreateWidgetUseCase) Execute(ctx context.Context, in Input) (Output, error)
//
// Needing a second method on a use case type is a signal it should split
// into two use cases. Use cases depend on repository interfaces defined
// next to them (or in internal/domain), never on concrete repository
// types, and may call other use cases to compose behavior instead of
// duplicating logic or reaching into another use case's repositories
// directly. See docs/microservice-standards.md §1.1 in the repo root for
// the full rules.
//
// Nothing lives here yet — this is a template. Add the first use case
// package once the service has a first business operation.
package usecase
