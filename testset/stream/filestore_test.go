package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	. "github.com/smartystreets/goconvey/convey"
)

// testDoc is a small JSON-object entity used to exercise the generic store and
// JSONCodec, independent of the query-specific wiring.
type testDoc struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Compile-time assurance that FileStore satisfies the full Stream contract
// (and therefore Reader and Writer, which Stream embeds).
var _ Stream[testDoc] = (*FileStore[testDoc])(nil)

func newTestReadFS() fstest.MapFS {
	return fstest.MapFS{
		"dir/a.json":     {Data: []byte(`{"id":"a","name":"Alpha"}`)},
		"dir/b.json":     {Data: []byte(`{"id":"b","name":"Beta"}`)},
		"dir/notes.txt":  {Data: []byte("not json")},
		"dir/bad.json":   {Data: []byte("{not json")},
		"dir/sub/c.json": {Data: []byte(`{"id":"c","name":"Gamma"}`)},
		"emptydir":       {Mode: fs.ModeDir},
	}
}

func TestJSONCodec(t *testing.T) {
	codec := JSONCodec[testDoc]()

	Convey("Given a JSONCodec", t, func() {
		Convey("When Decode is called with valid JSON", func() {
			item, err := codec.Decode("a", []byte(`{"id":"a","name":"Alpha"}`))

			Convey("Then it should return the decoded item with no error", func() {
				So(err, ShouldBeNil)
				So(item, ShouldResemble, testDoc{ID: "a", Name: "Alpha"})
			})
		})

		Convey("When Decode is called with invalid JSON", func() {
			_, err := codec.Decode("a", []byte("{not json"))

			Convey("Then it should return an error", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When Encode then Decode is called (round-trip)", func() {
			original := testDoc{ID: "x", Name: "Xray"}
			data, err := codec.Encode(original)
			So(err, ShouldBeNil)

			Convey("Then the output should be valid JSON that decodes back equal", func() {
				So(json.Valid(data), ShouldBeTrue)
				back, err := codec.Decode("x", data)
				So(err, ShouldBeNil)
				So(back, ShouldResemble, original)
			})
		})
	})
}

func TestFileStoreList(t *testing.T) {
	Convey("Given a FileStore over a filesystem with only valid .json files", t, func() {
		fsys := fstest.MapFS{
			"dir/a.json":     {Data: []byte(`{"id":"a","name":"Alpha"}`)},
			"dir/b.json":     {Data: []byte(`{"id":"b","name":"Beta"}`)},
			"dir/notes.txt":  {Data: []byte("not json")},
			"dir/sub/c.json": {Data: []byte(`{"id":"c","name":"Gamma"}`)},
		}
		store := NewFileStore(fsys, "dir", "", JSONCodec[testDoc]())

		Convey("When List is called", func() {
			items, err := store.List(context.Background())

			Convey("Then it should skip non-.json files and subdirectories", func() {
				So(err, ShouldBeNil)
				So(items, ShouldHaveLength, 2)

				ids := map[string]bool{}
				for _, it := range items {
					ids[it.ID] = true
				}
				So(ids["a"], ShouldBeTrue)
				So(ids["b"], ShouldBeTrue)
				So(ids["c"], ShouldBeFalse)
			})
		})
	})

	Convey("Given a FileStore pointed at an empty directory", t, func() {
		store := NewFileStore(newTestReadFS(), "emptydir", "", JSONCodec[testDoc]())

		Convey("When List is called", func() {
			items, err := store.List(context.Background())

			Convey("Then it should return an empty slice with no error", func() {
				So(err, ShouldBeNil)
				So(items, ShouldBeEmpty)
			})
		})
	})

	Convey("Given a FileStore pointed at a missing directory", t, func() {
		store := NewFileStore(newTestReadFS(), "nonexistent", "", JSONCodec[testDoc]())

		Convey("When List is called", func() {
			_, err := store.List(context.Background())

			Convey("Then it should return an error", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})

	Convey("Given a FileStore whose directory contains an undecodable .json file", t, func() {
		store := NewFileStore(newTestReadFS(), "dir", "", JSONCodec[testDoc]())

		Convey("When List is called", func() {
			_, err := store.List(context.Background())

			Convey("Then it should surface the decode error", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "decode")
			})
		})
	})
}

func TestFileStoreGet(t *testing.T) {
	Convey("Given a FileStore over an in-memory filesystem", t, func() {
		store := NewFileStore(newTestReadFS(), "dir", "", JSONCodec[testDoc]())

		Convey("When Get is called with an existing id", func() {
			item, err := store.Get(context.Background(), "a")

			Convey("Then it should return the decoded item with no error", func() {
				So(err, ShouldBeNil)
				So(item, ShouldResemble, testDoc{ID: "a", Name: "Alpha"})
			})
		})

		Convey("When Get is called with a missing id", func() {
			_, err := store.Get(context.Background(), "does-not-exist")

			Convey("Then it should return an error", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When Get is called for a file the codec cannot decode", func() {
			_, err := store.Get(context.Background(), "bad")

			Convey("Then it should return a decode error", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "decode")
			})
		})

		Convey("When Get is called with an invalid id", func() {
			// "sub/c" maps to dir/sub/c.json, which exists; rejecting it proves
			// the id check runs before any read and blocks path traversal.
			for _, id := range []string{"", ".", "..", "a/b", `a\b`, "sub/c"} {
				Convey(fmt.Sprintf("Then id %q should be rejected before any read", id), func() {
					_, err := store.Get(context.Background(), id)
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, "invalid item id")
				})
			}
		})
	})
}

func TestFileStorePut(t *testing.T) {
	Convey("Given a writable FileStore over a temp directory", t, func() {
		dir := t.TempDir()
		store := NewFileStore(os.DirFS(dir), ".", dir, JSONCodec[testDoc]())
		doc := testDoc{ID: "x", Name: "Xray"}

		Convey("When Put is called", func() {
			err := store.Put(context.Background(), "x", doc)

			Convey("Then it should write <id>.json whose bytes equal the codec output", func() {
				So(err, ShouldBeNil)

				written := filepath.Join(dir, "x.json")
				onDisk, statErr := os.ReadFile(written)
				So(statErr, ShouldBeNil)

				expected, encErr := JSONCodec[testDoc]().Encode(doc)
				So(encErr, ShouldBeNil)
				So(onDisk, ShouldResemble, expected)
			})

			Convey("Then the item can be read back (round-trip)", func() {
				So(err, ShouldBeNil)
				got, getErr := store.Get(context.Background(), "x")
				So(getErr, ShouldBeNil)
				So(got, ShouldResemble, doc)
			})
		})

		Convey("When Put is called with an invalid id", func() {
			// A writable store is required: the read-only guard precedes the id
			// check, so the validation is only reachable here.
			for _, id := range []string{"", ".", "..", "a/b", `a\b`} {
				Convey(fmt.Sprintf("Then id %q should be rejected", id), func() {
					err := store.Put(context.Background(), id, doc)
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, "invalid item id")
				})
			}
		})
	})

	Convey("Given a writable FileStore whose write directory does not yet exist", t, func() {
		dir := filepath.Join(t.TempDir(), "nested", "deep")
		store := NewFileStore(os.DirFS(t.TempDir()), ".", dir, JSONCodec[testDoc]())

		Convey("When Put is called", func() {
			err := store.Put(context.Background(), "y", testDoc{ID: "y", Name: "Yankee"})

			Convey("Then it should create the directory and write the file", func() {
				So(err, ShouldBeNil)
				_, statErr := os.Stat(filepath.Join(dir, "y.json"))
				So(statErr, ShouldBeNil)
			})
		})
	})

	Convey("Given a read-only FileStore (no write directory)", t, func() {
		store := NewFileStore(newTestReadFS(), "dir", "", JSONCodec[testDoc]())

		Convey("When Put is called", func() {
			err := store.Put(context.Background(), "z", testDoc{ID: "z"})

			Convey("Then it should return a read-only error", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "read-only")
			})
		})
	})
}

func TestFileStoreDelete(t *testing.T) {
	Convey("Given a writable FileStore with multiple items", t, func() {
		dir := t.TempDir()
		store := NewFileStore(os.DirFS(dir), ".", dir, JSONCodec[testDoc]())
		for _, item := range []testDoc{
			{ID: "delete-me", Name: "Delete me"},
			{ID: "keep-me", Name: "Keep me"},
		} {
			So(store.Put(context.Background(), item.ID, item), ShouldBeNil)
		}

		Convey("When Delete is called for an existing item", func() {
			err := store.Delete(context.Background(), "delete-me")

			Convey("Then it removes only that item's file", func() {
				So(err, ShouldBeNil)
				_, statErr := os.Stat(filepath.Join(dir, "delete-me.json"))
				So(os.IsNotExist(statErr), ShouldBeTrue)
				_, statErr = os.Stat(filepath.Join(dir, "keep-me.json"))
				So(statErr, ShouldBeNil)
			})
		})

		Convey("When Delete is called with an invalid id", func() {
			for _, id := range []string{"", ".", "..", "a/b", `a\b`} {
				Convey(fmt.Sprintf("Then id %q should be rejected", id), func() {
					err := store.Delete(context.Background(), id)
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, "invalid item id")
				})
			}
		})

		Convey("When Delete is called for a missing item", func() {
			err := store.Delete(context.Background(), "missing")

			Convey("Then it returns a wrapped delete error", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, `failed to delete item "missing"`)
			})
		})
	})

	Convey("Given a read-only FileStore", t, func() {
		store := NewFileStore(newTestReadFS(), "dir", "", JSONCodec[testDoc]())

		Convey("When Delete is called", func() {
			err := store.Delete(context.Background(), "a")

			Convey("Then it returns a read-only error", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "read-only")
			})
		})
	})
}
