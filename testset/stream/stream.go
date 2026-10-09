// Package stream provides a storage neutral interface for loading the
// repository's test-data items into memory, to be analysed or indexed into
// Elasticsearch.
//
// The store is described in generic terms ([Reader], [Writer], [Stream]) so
// that callers depend on the behaviour, not the backing.
package stream

// Stream combines read and write access to a store of items of type T.
//
//go:generate moq -out mocks/stream.go -pkg mocks . Stream
type Stream[T any] interface {
	Reader[T]
	Writer[T]
}
