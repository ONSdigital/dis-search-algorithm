package stream

import "context"

// Reader retrieves test-data items of type T from a store into memory.
// The interface is deliberately storage neutral.
//
//go:generate moq -out mocks/reader.go -pkg mocks . Reader
type Reader[T any] interface {
	// Get returns the single item identified by id, or an error if no such
	// item exists in the store.
	Get(ctx context.Context, id string) (T, error)

	// List returns every item held by the store.
	List(ctx context.Context) ([]T, error)
}
