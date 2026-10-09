package stream

import "context"

// Writer persists test-data items of type T to a store. It is storage neutral.
//
//go:generate moq -out mocks/writer.go -pkg mocks . Writer
type Writer[T any] interface {
	// Put creates or replaces the item stored under id.
	Put(ctx context.Context, id string, item T) error

	// Delete removes an item stored under the id.
	Delete(ctx context.Context, id string) error
}
