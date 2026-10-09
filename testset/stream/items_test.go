package stream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ONSdigital/dis-search-algorithm/testset"
	. "github.com/smartystreets/goconvey/convey"
)

// Compile-time assurance that the file-backed store satisfies the full Stream
// contract for the Item type.
var _ Stream[Item] = (*FileStore[Item])(nil)

func TestItemCodec(t *testing.T) {
	Convey("Given the itemCodec", t, func() {
		Convey("When Decode is called with an id and body bytes", func() {
			body := []byte(`{"query":{"match_all":{}}}`)
			item, err := itemCodec.Decode("browse", body)

			Convey("Then it should set Name from the id and Body from the bytes", func() {
				So(err, ShouldBeNil)
				So(item.Name, ShouldEqual, "browse")
				So(string(item.Body), ShouldEqual, string(body))
			})

			Convey("Then mutating the source bytes should not affect the stored Body", func() {
				So(err, ShouldBeNil)
				body[0] = 'X' // corrupt the caller's slice after decode
				So(string(item.Body), ShouldEqual, `{"query":{"match_all":{}}}`)
			})
		})

		Convey("When Decode is called with indented JSON", func() {
			item, err := itemCodec.Decode("browse", []byte("{\n  \"size\": 10\n}"))

			Convey("Then the Body should be minified", func() {
				So(err, ShouldBeNil)
				So(string(item.Body), ShouldEqual, `{"size":10}`)
			})
		})

		Convey("When Decode is called with invalid JSON", func() {
			_, err := itemCodec.Decode("bad", []byte("{not json"))

			Convey("Then it should return an error", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When Encode is called with an Item with a valid body", func() {
			body := json.RawMessage(`{"size":10}`)
			data, err := itemCodec.Encode(Item{Name: "n", Body: body})

			Convey("Then it should return the body bytes verbatim", func() {
				So(err, ShouldBeNil)
				So(string(data), ShouldEqual, string(body))
			})
		})

		Convey("When Encode is called with an Item with an invalid body", func() {
			_, err := itemCodec.Encode(Item{Name: "n", Body: json.RawMessage("{not json")})

			Convey("Then it should return an error", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func TestStores(t *testing.T) {
	cases := []struct {
		name  string
		store Stream[Item]
	}{
		{
			name:  "document",
			store: NewDocumentStore(),
		},
		{
			name:  "judgement",
			store: NewJudgementStore(),
		},
		{
			name:  "term",
			store: NewTermStore(),
		},
	}

	for _, tc := range cases {
		Convey(fmt.Sprintf("Given a %s store over the embedded fixtures", tc.name), t, func() {
			store := tc.store

			Convey("When List is called", func() {
				items, err := store.List(context.Background())

				Convey("Then it should return non-empty items with valid, minified bodies", func() {
					So(err, ShouldBeNil)
					So(items, ShouldNotBeEmpty)

					for _, item := range items {
						So(item.Name, ShouldNotBeEmpty)
						So(json.Valid(item.Body), ShouldBeTrue)
						// Fixtures are stored indented on disk; the loaded body is
						// minified, so it must contain no newlines.
						So(bytes.Contains(item.Body, []byte("\n")), ShouldBeFalse)
					}
				})
			})

			Convey("When Get is called with a name returned by List", func() {
				items, listErr := store.List(context.Background())
				var item Item
				var getErr error
				if listErr == nil && len(items) > 0 {
					item, getErr = store.Get(context.Background(), items[0].Name)
				}

				Convey("Then it should return that item with a valid body", func() {
					So(listErr, ShouldBeNil)
					So(items, ShouldNotBeEmpty)
					if listErr == nil && len(items) > 0 {
						So(getErr, ShouldBeNil)
						So(item.Name, ShouldEqual, items[0].Name)
						So(json.Valid(item.Body), ShouldBeTrue)
					}
				})
			})

			Convey("When Get is called with a name that does not exist", func() {
				_, err := store.Get(context.Background(), "does-not-exist")

				Convey("Then it should return an error", func() {
					So(err, ShouldNotBeNil)
				})
			})
		})
	}
}

func TestStoreWriteDir(t *testing.T) {
	Convey("Given a document store whose writes are redirected to a temp dir", t, func() {
		dir := t.TempDir()
		store := NewDocumentStoreWithWriteDir(dir)
		embeddedItems, embeddedErr := store.List(context.Background())

		Convey("When an item is Put", func() {
			err := store.Put(context.Background(), "probe",
				Item{Name: "probe", Body: json.RawMessage(`{"query":{"match_all":{}}}`)})

			Convey("Then the file should land under the temp dir, not the fixtures", func() {
				So(err, ShouldBeNil)
				_, statErr := os.Stat(filepath.Join(dir, "probe.json"))
				So(statErr, ShouldBeNil)
			})

			Convey("Then the embedded reads should not see the on-disk write", func() {
				So(err, ShouldBeNil)
				_, getErr := store.Get(context.Background(), "probe")
				So(getErr, ShouldNotBeNil)
			})
		})

		Convey("When List is called", func() {
			items, err := store.List(context.Background())

			Convey("Then it should still return the embedded fixtures", func() {
				So(embeddedErr, ShouldBeNil)
				So(err, ShouldBeNil)
				So(embeddedItems, ShouldNotBeEmpty)
				So(items, ShouldResemble, embeddedItems)
			})
		})
	})
}

func TestItemFileStore(t *testing.T) {
	Convey("Given an on-disk item store over a temp directory", t, func() {
		dir := t.TempDir()
		store := NewItemFileStore(dir)
		item := Item{Name: "custom_probe", Body: json.RawMessage(`{"query":{"match_all":{}}}`)}

		Convey("When an item is written then read back", func() {
			putErr := store.Put(context.Background(), item.Name, item)
			got, getErr := store.Get(context.Background(), item.Name)

			Convey("Then it should round-trip on Name and Body", func() {
				So(putErr, ShouldBeNil)
				So(getErr, ShouldBeNil)
				So(got.Name, ShouldEqual, item.Name)
				So(string(got.Body), ShouldEqual, string(item.Body))
			})

			Convey("Then the written file content should equal the body bytes", func() {
				So(putErr, ShouldBeNil)
				onDisk, readErr := os.ReadFile(filepath.Join(dir, "custom_probe.json"))
				So(readErr, ShouldBeNil)
				So(string(onDisk), ShouldEqual, string(item.Body))
			})

			Convey("Then List should include the written item", func() {
				So(putErr, ShouldBeNil)
				items, listErr := store.List(context.Background())
				So(listErr, ShouldBeNil)
				So(items, ShouldHaveLength, 1)
				So(items[0].Name, ShouldEqual, item.Name)
			})
		})
	})
}

// --- test-only store constructors ---------------------------------------
// These variants exist purely to exercise the write path without touching the
// real testset fixtures, so they live with the tests rather than in the
// package's public surface. NewDocumentStore/NewJudgementStore/NewTermStore are
// the production entry points and stay in items.go.
//-------------------------------------

// NewDocumentStoreWithWriteDir is like NewDocumentStore - reads still come from
// the embedded fixtures - but Put writes to writeDir instead of the on-disk
// fixtures. Pass t.TempDir() so writes never touch testset/documents. Because
// reads remain served from the embed, an item written here lands on disk but is
// not visible to this store's Get or List; use NewItemFileStore when a write
// must round-trip within a run.
func NewDocumentStoreWithWriteDir(writeDir string) *FileStore[Item] {
	return NewFileStore(
		testset.DocumentFixturesFS(),
		testset.DocumentFixturesDir,
		writeDir,
		itemCodec,
	)
}

// NewItemFileStore returns an item store that both reads and writes the on-disk
// directory dir (no embedding), so Put is immediately visible to Get and List.
func NewItemFileStore(dir string) *FileStore[Item] {
	return NewFileStore(os.DirFS(dir), ".", dir, itemCodec)
}
