package app

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ONSdigital/dis-search-algorithm/testset/stream"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

func TestReadTermsCSV(t *testing.T) {
	Convey("Given a CSV with reordered columns and surrounding whitespace", t, func() {
		input := "description,query,query_id\n  A description  ,  a query  ,  term-a  \n"

		Convey("When the terms are read", func() {
			rows, err := readTermsCSV(strings.NewReader(input))

			Convey("Then the values are trimmed and mapped by header", func() {
				So(err, ShouldBeNil)
				So(rows, ShouldResemble, []term{{ID: "term-a", Query: "a query", Description: "A description"}})
			})
		})
	})

	Convey("Given invalid term CSV input", t, func() {
		tests := []struct {
			name string
			csv  string
			want string
		}{
			{name: "empty CSV", csv: "", want: "CSV is empty"},
			{name: "missing query ID header", csv: "query,description\na,b\n", want: csvColumnQueryID},
			{name: "missing query header", csv: "query_id,description\na,b\n", want: csvColumnQuery},
			{name: "missing description header", csv: "query_id,query\na,b\n", want: csvColumnDescription},
			{name: "short row", csv: "query_id,query,description\na,b\n", want: "row 2: expected at least 3 columns"},
			{name: "empty required field", csv: "query_id,query,description\na,,b\n", want: "must not be empty"},
			{name: "invalid CSV", csv: "query_id,query,description\n\"unterminated", want: "failed to read CSV"},
		}
		for _, tc := range tests {
			tc := tc
			Convey("When "+tc.name, func() {
				_, err := readTermsCSV(strings.NewReader(tc.csv))

				Convey("Then it reports the input problem", func() {
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, tc.want)
				})
			})
		}
	})
}

func TestMarshalTerm(t *testing.T) {
	Convey("Given a term containing JSON-sensitive characters", t, func() {
		want := term{ID: "id\"1", Query: "line 1\nline 2", Description: `a \ "description"`}

		Convey("When it is marshalled", func() {
			body, err := marshalTerm(want)

			Convey("Then it is valid, pretty-printed JSON with a trailing newline", func() {
				So(err, ShouldBeNil)
				So(strings.HasSuffix(string(body), "\n"), ShouldBeTrue)
				var got term
				So(json.Unmarshal(body, &got), ShouldBeNil)
				So(got, ShouldResemble, want)
			})
		})
	})
}

func TestWriteAndReadTermsCSV(t *testing.T) {
	Convey("Given terms with commas and newlines", t, func() {
		want := []term{
			{ID: "term-a", Query: "a, quoted query", Description: "line 1\nline 2"},
			{ID: "term-b", Query: "another query", Description: "description"},
		}

		Convey("When terms are written and read back", func() {
			var output strings.Builder
			err := writeTermsCSV(&output, want)
			So(err, ShouldBeNil)
			got, err := readTermsCSV(strings.NewReader(output.String()))

			Convey("Then the terms round-trip unchanged", func() {
				So(err, ShouldBeNil)
				So(got, ShouldResemble, want)
			})
		})
	})
}

func TestGetTerms(t *testing.T) {
	Convey("Given a term store containing valid JSON", t, func() {
		want := term{ID: "term-a", Query: "a query", Description: "a description"}
		body, err := json.Marshal(want)
		So(err, ShouldBeNil)
		app := &App{Terms: fakeStore{items: []stream.Item{{Name: want.ID, Body: body}}}}

		Convey("When all terms are retrieved", func() {
			got, err := app.GetTerms(context.Background())

			Convey("Then the stored terms are decoded", func() {
				So(err, ShouldBeNil)
				So(got, ShouldResemble, []term{want})
			})
		})
	})

	Convey("Given a term store that fails to list", t, func() {
		app := &App{Terms: fakeStore{listErr: errors.New(errDiskError)}}

		Convey("When all terms are retrieved", func() {
			_, err := app.GetTerms(context.Background())

			Convey("Then the list error is wrapped", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "failed to list terms")
			})
		})
	})

	Convey("Given a term store containing invalid JSON", t, func() {
		app := &App{Terms: fakeStore{items: []stream.Item{{Name: "broken", Body: []byte("{")}}}}

		Convey("When all terms are retrieved", func() {
			_, err := app.GetTerms(context.Background())

			Convey("Then the invalid item is identified", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, `failed to parse term "broken"`)
			})
		})
	})
}

func TestImportTermsCSV(t *testing.T) {
	termItem := func(value term) stream.Item {
		body, err := json.Marshal(value)
		So(err, ShouldBeNil)
		return stream.Item{Name: value.ID, Body: body}
	}

	Convey("Given existing terms and an incoming CSV snapshot", t, func() {
		unchanged := term{ID: "same", Query: "keep", Description: "same description"}
		toUpdate := term{ID: "update", Query: "old query", Description: "description"}
		toDelete := term{ID: "delete", Query: "remove", Description: "description"}
		var puts []stream.Item
		var deletes []string
		app := &App{Terms: fakeStore{
			items:    []stream.Item{termItem(unchanged), termItem(toUpdate), termItem(toDelete)},
			putCalls: &puts, deleteCalls: &deletes,
		}}
		input := "query_id,query,description\nsame,keep,same description\nupdate,new query,description\nnew,brand new,description\n"

		Convey("When the CSV is imported", func() {
			err := app.importTermsCSV(context.Background(), strings.NewReader(input))

			Convey("Then changed terms are written and absent terms are deleted", func() {
				So(err, ShouldBeNil)
				So(puts, ShouldHaveLength, 2)
				So([]string{puts[0].Name, puts[1].Name}, ShouldResemble, []string{"update", "new"})
				So(deletes, ShouldResemble, []string{"delete"})
			})
		})
	})

	Convey("Given a term store that fails to write", t, func() {
		app := &App{Terms: fakeStore{putErr: errors.New(errDiskError)}}

		Convey("When an incoming term is imported", func() {
			err := app.importTermsCSV(context.Background(), strings.NewReader("query_id,query,description\nnew,query,description\n"))

			Convey("Then the write error is wrapped", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "failed to write term")
			})
		})
	})

	Convey("Given a term store that fails to delete", t, func() {
		item := termItem(term{ID: "delete", Query: "query", Description: "description"})
		app := &App{Terms: fakeStore{items: []stream.Item{item}, deleteErr: errors.New(errDiskError)}}

		Convey("When an empty snapshot is imported", func() {
			err := app.importTermsCSV(context.Background(), strings.NewReader("query_id,query,description\n"))

			Convey("Then the delete error identifies the term", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, `failed to delete term "delete"`)
			})
		})
	})
}

func TestImportTermsAndExportTerms(t *testing.T) {
	Convey("Given a term app and blank paths", t, func() {
		app := &App{Terms: fakeStore{}}

		Convey("When terms are imported or exported", func() {
			importErr := app.ImportTerms(context.Background(), " ")
			exportErr := app.ExportTerms(context.Background(), " ")

			Convey("Then both operations reject the blank path", func() {
				So(importErr, ShouldNotBeNil)
				So(exportErr, ShouldNotBeNil)
			})
		})
	})

	Convey("Given a CSV file and a store containing the same term", t, func() {
		dir := t.TempDir()
		inputPath := filepath.Join(dir, "terms.csv")
		outputPath := filepath.Join(dir, "export.csv")
		So(os.WriteFile(inputPath, []byte("query_id,query,description\nterm-a,query,description\n"), 0o600), ShouldBeNil)
		itemBody, err := json.Marshal(term{ID: "term-a", Query: "query", Description: "description"})
		So(err, ShouldBeNil)
		app := &App{Terms: fakeStore{items: []stream.Item{{Name: "term-a", Body: itemBody}}}}

		Convey("When the file is imported and terms are exported", func() {
			importErr := app.ImportTerms(context.Background(), inputPath)
			exportErr := app.ExportTerms(context.Background(), outputPath)

			Convey("Then the unchanged term is exported as CSV", func() {
				So(importErr, ShouldBeNil)
				So(exportErr, ShouldBeNil)
				data, readErr := os.ReadFile(outputPath)
				So(readErr, ShouldBeNil)
				rows, parseErr := csv.NewReader(strings.NewReader(string(data))).ReadAll()
				So(parseErr, ShouldBeNil)
				So(rows, ShouldResemble, [][]string{
					{"query_id", "query", "description"},
					{"term-a", "query", "description"},
				})
			})
		})
	})
}
