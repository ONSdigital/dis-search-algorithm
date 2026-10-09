package stream

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

const (
	// File permission constants for written items.

	itemDirMode  = 0o755 // rwxr-xr-x - directory holding test data
	itemFileMode = 0o644 // rw-r--r-- - individual test-data files
)

// Codec maps a stored item of type T to and from the raw bytes of its file.
//
// It lets a single [FileStore] serve entities whose file representation
// differs: a document, judgement or term file *is* its body, stored verbatim
// with its id taken from the filename (see [itemCodec]), whereas an entity
// persisted as a typed JSON object is decoded into T with its id carried inside
// the object (see [JSONCodec]). The store owns filesystem iteration and naming;
// the codec owns the bytes<->item translation.
type Codec[T any] struct {
	// Decode builds an item from its id (the filename without the .json
	// extension) and the raw file bytes.
	Decode func(id string, data []byte) (T, error)
	// Encode returns the bytes to persist for an item.
	Encode func(item T) ([]byte, error)
}

// JSONCodec returns a [Codec] that unmarshalls each file as JSON into T and
// marshals items back as indented JSON. Suitable for entities stored as JSON
// objects (e.g. documents, judgements in the testset/ directory); the id is
// not used during decode because it is carried inside the object.
func JSONCodec[T any]() Codec[T] {
	return Codec[T]{
		Decode: func(_ string, data []byte) (T, error) {
			var item T
			if err := json.Unmarshal(data, &item); err != nil {
				return item, err
			}
			return item, nil
		},
		Encode: func(item T) ([]byte, error) {
			return json.MarshalIndent(item, "", "  ")
		},
	}
}

// FileStore is a filesystem-backed [Stream] holding one file per item, named
// "<id>.json" (the file's name is its id).
//
// Reads and writes are intentionally decoupled so the same type can serve a
// read-only embedded snapshot and a writable on-disk directory:
//
//   - readFS is any [fs.FS]; an [embed.FS] gives a compiled-in, read-only
//     snapshot (see [NewDocumentStore]); os.DirFS gives live reads from disk.
//   - writeDir is an os path that Put writes to. Because an embed.FS cannot be
//     written to, Put targets this real directory instead; the newly written
//     file is only picked up by an embed-backed reader on the next build.
//
// The codec translates between item and file bytes. FileStore satisfies
// Stream[T] interface.
type FileStore[T any] struct {
	readFS   fs.FS    // source for Get/List; paths are relative to its root
	readDir  string   // subdirectory within readFS to read from
	writeDir string   // os directory Put writes to; empty disables writes
	codec    Codec[T] // item <-> file-bytes translation
}

// NewFileStore constructs a FileStore reading items from readDir within readFS
// and writing them to the os directory writeDir, using codec to translate
// between items and file bytes. Pass the same logical path for readDir and
// writeDir when the read source and write target are the same directory; pass
// an empty writeDir for a read-only store.
func NewFileStore[T any](readFS fs.FS, readDir, writeDir string, codec Codec[T]) *FileStore[T] {
	return &FileStore[T]{
		readFS:   readFS,
		readDir:  readDir,
		writeDir: writeDir,
		codec:    codec,
	}
}

// Get returns the item stored under id, or an error if it cannot be found or
// decoded.
func (s *FileStore[T]) Get(_ context.Context, id string) (T, error) {
	var zero T

	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\\`) {
		return zero, errors.Errorf("invalid item id %q", id)
	}

	fileBytes, err := fs.ReadFile(s.readFS, path.Join(s.readDir, id+".json"))
	if err != nil {
		return zero, errors.Wrapf(err, "failed to read item %q", id)
	}

	item, err := s.codec.Decode(id, fileBytes)
	if err != nil {
		return zero, errors.Wrapf(err, "failed to decode item %q", id)
	}

	return item, nil
}

// List returns every item in the store's read directory, one per ".json" file.
func (s *FileStore[T]) List(_ context.Context) ([]T, error) {
	dirEntries, err := fs.ReadDir(s.readFS, s.readDir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read items directory %q", s.readDir)
	}

	items := make([]T, 0, len(dirEntries))
	for _, dirEntry := range dirEntries {
		// skip directories and non-JSON files
		if dirEntry.IsDir() || filepath.Ext(dirEntry.Name()) != ".json" {
			continue
		}

		fileBytes, err := fs.ReadFile(s.readFS, path.Join(s.readDir, dirEntry.Name()))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read item file %q", dirEntry.Name())
		}

		id := strings.TrimSuffix(dirEntry.Name(), ".json")
		item, err := s.codec.Decode(id, fileBytes)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode item file %q", dirEntry.Name())
		}

		items = append(items, item)
	}

	return items, nil
}

// Put creates or replaces the item stored under id by encoding it and
// writing the bytes to writeDir/<id>.json.
//
// Put writes to the os filesystem. When the store's reads are served from an
// embed.FS (see [NewDocumentStore]) the written file will not appear in
// Get/List results until it is embedded on the next build.
func (s *FileStore[T]) Put(_ context.Context, id string, item T) error {
	if s.writeDir == "" {
		return errors.Errorf("store is read-only: cannot write item %q", id)
	}

	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\\`) {
		return errors.Errorf("invalid item id %q", id)
	}

	fileBytes, err := s.codec.Encode(item)
	if err != nil {
		return errors.Wrapf(err, "failed to encode item %q", id)
	}

	if err := os.MkdirAll(s.writeDir, itemDirMode); err != nil {
		return errors.Wrapf(err, "failed to create write directory %q", s.writeDir)
	}

	if err := os.WriteFile(filepath.Join(s.writeDir, id+".json"), fileBytes, itemFileMode); err != nil {
		return errors.Wrapf(err, "failed to write item %q", id)
	}

	return nil
}

// Delete removes the item stored under id from the write directory.
func (s *FileStore[T]) Delete(_ context.Context, id string) error {
	if s.writeDir == "" {
		return errors.Errorf("store is read-only: cannot delete item %q", id)
	}

	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\\`) {
		return errors.Errorf("invalid item id %q", id)
	}

	if err := os.Remove(filepath.Join(s.writeDir, id+".json")); err != nil {
		return errors.Wrapf(err, "failed to delete item %q", id)
	}

	return nil
}
