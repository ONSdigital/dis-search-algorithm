package app

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ONSdigital/dis-search-algorithm/testset/stream"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestWriteEvaluationsCSV(t *testing.T) {
	Convey("Given evaluations containing multiple documents", t, func() {
		evaluations := []termEvaluation{{
			Term: term{ID: docNameCPI, Query: testExportQuery},
			Hits: []evaluatedHit{
				{
					DocumentID: docNameCPI,
					Rank:       1,
					Relevance:  4,
					Judged:     true,
					Title:      testCPITitle,
					URI:        testCPIURI,
				},
				{
					DocumentID: docNameGrowth,
					Rank:       2,
					Relevance:  0,
					Judged:     false,
					Title:      testGrowthTitle,
					URI:        testGrowthURI,
				},
			},
		}}
		var output bytes.Buffer

		Convey("When the evaluations are written as CSV", func() {
			err := writeEvaluationsCSV(&output, evaluations)

			Convey("Then the header and ranked rows should be written in order", func() {
				So(err, ShouldBeNil)
				records, readErr := csv.NewReader(&output).ReadAll()
				So(readErr, ShouldBeNil)
				So(records, ShouldResemble, [][]string{
					{
						csvColumnQueryID,
						csvColumnQuery,
						csvColumnDocumentID,
						csvColumnCurrentRelevance,
						csvColumnTitle,
						csvColumnURI,
					},
					{
						docNameCPI,
						testExportQuery,
						docNameCPI,
						"4",
						testCPITitle,
						testCPIURI,
					},
					{
						docNameCPI,
						testExportQuery,
						docNameGrowth,
						"0",
						testGrowthTitle,
						testGrowthURI,
					},
				})
			})
		})
	})

	Convey("Given a writer that fails", t, func() {
		Convey("When CSV output is written", func() {
			err := writeEvaluationsCSV(failingWriter{}, nil)

			Convey("Then the write error should be returned", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "failed to flush CSV")
			})
		})
	})
}

const (
	docNameAccountancy = "accountancy-services-timeseries"
	orphanDocID        = "unknown-doc"
	csvHeader          = "query_id,query,doc_id,current_relevance,title,uri"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

// csvBody assembles an export-format CSV from the shared header and the given
// data rows.
func csvBody(rows ...string) string {
	lines := append([]string{csvHeader}, rows...)
	return strings.Join(lines, "\n") + "\n"
}

// documentItems builds a corpus of documents identified only by id (import only
// reads the ids).
func documentItems(ids ...string) []stream.Item {
	items := make([]stream.Item, 0, len(ids))
	for _, id := range ids {
		items = append(items, stream.Item{Name: id, Body: []byte(`{}`)})
	}
	return items
}

// judgementItem builds a stored judgement item for the Judgements fake store,
// serialised independently of the code under test.
func judgementItem(queryID string, entries ...judgementEntry) stream.Item {
	body, err := json.Marshal(judgement{QueryID: queryID, Judgements: entries})
	if err != nil {
		panic(err)
	}
	return stream.Item{Name: queryID, Body: body}
}

func TestReadRelevanceCSV(t *testing.T) {
	Convey("Given an export-format CSV", t, func() {
		Convey("When it is parsed", func() {
			rows, err := readRelevanceCSV(strings.NewReader(csvBody(
				"cpi-latest,cpi latest,cpi-latest,4,Title,/cpi",
				"cpi-latest,cpi latest,growth-dataset,0,Growth,/growth",
			)))

			Convey("Then only query_id, doc_id and current_relevance are kept", func() {
				So(err, ShouldBeNil)
				So(rows, ShouldResemble, []relevanceRow{
					{QueryID: docNameCPI, DocID: docNameCPI, Relevance: 4},
					{QueryID: docNameCPI, DocID: docNameGrowth, Relevance: 0},
				})
			})
		})
	})

	Convey("Given a CSV with reordered and extra columns", t, func() {
		Convey("When it is parsed", func() {
			rows, err := readRelevanceCSV(strings.NewReader(
				"extra,doc_id,current_relevance,query_id\n" +
					"junk, cpi-latest , 3 , cpi-latest \n",
			))

			Convey("Then columns are located by name and values trimmed", func() {
				So(err, ShouldBeNil)
				So(rows, ShouldResemble, []relevanceRow{
					{QueryID: docNameCPI, DocID: docNameCPI, Relevance: 3},
				})
			})
		})
	})

	Convey("Given a CSV missing a required column", t, func() {
		Convey("When it is parsed", func() {
			_, err := readRelevanceCSV(strings.NewReader("query_id,query,current_relevance\ncpi-latest,cpi,4\n"))

			Convey("Then it reports the missing column", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, `missing required column "doc_id"`)
			})
		})
	})

	Convey("Given a CSV with a non-integer relevance", t, func() {
		Convey("When it is parsed", func() {
			_, err := readRelevanceCSV(strings.NewReader(csvBody("cpi-latest,cpi,cpi-latest,high,Title,/cpi")))

			Convey("Then it reports an invalid relevance", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "invalid current_relevance")
			})
		})
	})

	Convey("Given a CSV with an out-of-range relevance", t, func() {
		Convey("When it is parsed", func() {
			_, err := readRelevanceCSV(strings.NewReader(csvBody("cpi-latest,cpi,cpi-latest,5,Title,/cpi")))

			Convey("Then it reports the range violation", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "out of range [0,4]")
			})
		})
	})

	Convey("Given a CSV row with an empty doc_id", t, func() {
		Convey("When it is parsed", func() {
			_, err := readRelevanceCSV(strings.NewReader(csvBody("cpi-latest,cpi,,4,Title,/cpi")))

			Convey("Then it rejects the empty identifier", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "must not be empty")
			})
		})
	})

	Convey("Given a header-only CSV", t, func() {
		Convey("When it is parsed", func() {
			rows, err := readRelevanceCSV(strings.NewReader(csvBody()))

			Convey("Then it yields no rows and no error", func() {
				So(err, ShouldBeNil)
				So(rows, ShouldBeEmpty)
			})
		})
	})

	Convey("Given a completely empty input", t, func() {
		Convey("When it is parsed", func() {
			_, err := readRelevanceCSV(strings.NewReader(""))

			Convey("Then it reports the missing header", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "CSV is empty")
			})
		})
	})

	Convey("Given a reader that fails", t, func() {
		Convey("When it is parsed", func() {
			_, err := readRelevanceCSV(failingReader{})

			Convey("Then the read error is returned", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "failed to read CSV")
			})
		})
	})
}

func TestMergeJudgement(t *testing.T) {
	Convey("Given an existing judgement", t, func() {
		existing := []judgementEntry{
			{DocID: docNameCPI, Relevance: 4},
			{DocID: docNameGrowth, Relevance: 2},
		}

		Convey("When a row updates one document", func() {
			result := mergeJudgement(existing, []relevanceRow{{DocID: docNameCPI, Relevance: 3}})

			Convey("Then that grade changes and the others are kept, sorted by doc_id", func() {
				So(result, ShouldResemble, []judgementEntry{
					{DocID: docNameCPI, Relevance: 3},
					{DocID: docNameGrowth, Relevance: 2},
				})
			})
		})

		Convey("When a row downgrades a document to 0", func() {
			result := mergeJudgement(existing, []relevanceRow{{DocID: docNameCPI, Relevance: 0}})

			Convey("Then its entry is removed (0 == unjudged)", func() {
				So(result, ShouldResemble, []judgementEntry{
					{DocID: docNameGrowth, Relevance: 2},
				})
			})
		})

		Convey("When a new document is scored 0", func() {
			result := mergeJudgement(existing, []relevanceRow{{DocID: orphanDocID, Relevance: 0}})

			Convey("Then nothing is added", func() {
				So(result, ShouldResemble, existing)
			})
		})
	})

	Convey("Given no existing judgement", t, func() {
		Convey("When duplicate rows score the same document", func() {
			result := mergeJudgement(nil, []relevanceRow{
				{DocID: docNameCPI, Relevance: 2},
				{DocID: docNameCPI, Relevance: 3},
			})

			Convey("Then the last row wins", func() {
				So(result, ShouldResemble, []judgementEntry{{DocID: docNameCPI, Relevance: 3}})
			})
		})

		Convey("When every row collapses to 0", func() {
			result := mergeJudgement(
				[]judgementEntry{{DocID: docNameCPI, Relevance: 4}},
				[]relevanceRow{{DocID: docNameCPI, Relevance: 0}},
			)

			Convey("Then the result is an empty, non-nil slice", func() {
				So(result, ShouldResemble, []judgementEntry{})
			})
		})
	})
}

func TestBuildJudgements(t *testing.T) {
	Convey("Given rows that mix corpus and non-corpus documents", t, func() {
		known := map[string]struct{}{docNameCPI: {}, docNameGrowth: {}}
		rows := []relevanceRow{
			{QueryID: docNameCPI, DocID: docNameCPI, Relevance: 3},
			{QueryID: docNameCPI, DocID: orphanDocID, Relevance: 2},
		}

		Convey("When the judgements are built", func() {
			updated, summary := buildJudgements(rows, known, nil)

			Convey("Then orphan rows are skipped and counted", func() {
				So(summary.Applied, ShouldEqual, 1)
				So(summary.Skipped, ShouldEqual, 1)
				So(updated[docNameCPI], ShouldResemble, []judgementEntry{{DocID: docNameCPI, Relevance: 3}})
			})
		})
	})

	Convey("Given a query reachable only through an orphan row", t, func() {
		known := map[string]struct{}{docNameCPI: {}}
		rows := []relevanceRow{{QueryID: "orphan-query", DocID: orphanDocID, Relevance: 3}}

		Convey("When the judgements are built", func() {
			updated, summary := buildJudgements(rows, known, nil)

			Convey("Then that query produces no update", func() {
				So(summary.Skipped, ShouldEqual, 1)
				So(updated, ShouldBeEmpty)
			})
		})
	})
}

func TestImportCSV(t *testing.T) {
	corpus := documentItems(docNameCPI, docNameGrowth, docNameAccountancy)

	Convey("Given a corpus and an existing answer key", t, func() {
		existing := judgementItem(docNameCPI,
			judgementEntry{DocID: docNameCPI, Relevance: 4},
			judgementEntry{DocID: docNameAccountancy, Relevance: 1},
			judgementEntry{DocID: docNameGrowth, Relevance: 2},
		)

		Convey("When a CSV re-scores one document", func() {
			var puts []stream.Item
			app := &App{
				Documents:  fakeStore{items: corpus},
				Judgements: fakeStore{items: []stream.Item{existing}, putCalls: &puts},
			}

			err := app.importCSV(context.Background(), strings.NewReader(csvBody(
				"cpi-latest,cpi latest,cpi-latest,3,Consumer prices,/cpi",
			)))

			Convey("Then the judgement is merged, sorted, and written pretty-printed", func() {
				So(err, ShouldBeNil)
				So(puts, ShouldHaveLength, 1)
				So(puts[0].Name, ShouldEqual, docNameCPI)
				So(strings.HasSuffix(string(puts[0].Body), "\n"), ShouldBeTrue)
				So(string(puts[0].Body), ShouldContainSubstring, "\n  \"query_id\":")

				var got judgement
				So(json.Unmarshal(puts[0].Body, &got), ShouldBeNil)
				So(got.QueryID, ShouldEqual, docNameCPI)
				So(got.Judgements, ShouldResemble, []judgementEntry{
					{DocID: docNameAccountancy, Relevance: 1},
					{DocID: docNameCPI, Relevance: 3},
					{DocID: docNameGrowth, Relevance: 2},
				})
			})
		})

		Convey("When a CSV re-states the current grades unchanged", func() {
			var puts []stream.Item
			app := &App{
				Documents:  fakeStore{items: corpus},
				Judgements: fakeStore{items: []stream.Item{existing}, putCalls: &puts},
			}

			err := app.importCSV(context.Background(), strings.NewReader(csvBody(
				"cpi-latest,cpi latest,cpi-latest,4,Consumer prices,/cpi",
				"cpi-latest,cpi latest,accountancy-services-timeseries,1,Accountancy,/acc",
				"cpi-latest,cpi latest,growth-dataset,2,Growth,/growth",
			)))

			Convey("Then nothing is written", func() {
				So(err, ShouldBeNil)
				So(puts, ShouldBeEmpty)
			})
		})

		Convey("When a CSV row references a document not in the corpus", func() {
			var puts []stream.Item
			app := &App{
				Documents:  fakeStore{items: corpus},
				Judgements: fakeStore{items: []stream.Item{existing}, putCalls: &puts},
			}

			err := app.importCSV(context.Background(), strings.NewReader(csvBody(
				"cpi-latest,cpi latest,cpi-latest,3,Consumer prices,/cpi",
				"cpi-latest,cpi latest,unknown-doc,4,Ghost,/ghost",
			)))

			Convey("Then the orphan row is skipped and not written", func() {
				So(err, ShouldBeNil)
				So(puts, ShouldHaveLength, 1)
				So(string(puts[0].Body), ShouldNotContainSubstring, orphanDocID)
			})
		})
	})

	Convey("Given a query with no existing judgement file", t, func() {
		var puts []stream.Item
		app := &App{
			Documents:  fakeStore{items: corpus},
			Judgements: fakeStore{items: nil, putCalls: &puts},
		}

		Convey("When a CSV scores it", func() {
			err := app.importCSV(context.Background(), strings.NewReader(csvBody(
				"cpi-latest,cpi latest,cpi-latest,3,Consumer prices,/cpi",
			)))

			Convey("Then a new answer key is created", func() {
				So(err, ShouldBeNil)
				So(puts, ShouldHaveLength, 1)
				var got judgement
				So(json.Unmarshal(puts[0].Body, &got), ShouldBeNil)
				So(got.Judgements, ShouldResemble, []judgementEntry{{DocID: docNameCPI, Relevance: 3}})
			})
		})
	})

	Convey("Given a header-only CSV", t, func() {
		var puts []stream.Item
		app := &App{
			Documents:  fakeStore{items: corpus},
			Judgements: fakeStore{items: nil, putCalls: &puts},
		}

		Convey("When it is imported", func() {
			err := app.importCSV(context.Background(), strings.NewReader(csvBody()))

			Convey("Then nothing is written", func() {
				So(err, ShouldBeNil)
				So(puts, ShouldBeEmpty)
			})
		})
	})

	Convey("Given the document store fails to list", t, func() {
		app := &App{
			Documents:  fakeStore{listErr: errors.New(errDiskError)},
			Judgements: fakeStore{},
		}

		Convey("When a CSV is imported", func() {
			err := app.importCSV(context.Background(), strings.NewReader(csvBody(
				"cpi-latest,cpi latest,cpi-latest,3,Consumer prices,/cpi",
			)))

			Convey("Then the error is wrapped and surfaced", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "failed to list documents")
			})
		})
	})

	Convey("Given the judgement store fails to write", t, func() {
		app := &App{
			Documents:  fakeStore{items: corpus},
			Judgements: fakeStore{putErr: errors.New(errConnRefused)},
		}

		Convey("When an import would change a judgement", func() {
			err := app.importCSV(context.Background(), strings.NewReader(csvBody(
				"cpi-latest,cpi latest,cpi-latest,3,Consumer prices,/cpi",
			)))

			Convey("Then the write error is wrapped and surfaced", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "failed to write judgement")
			})
		})
	})
}
