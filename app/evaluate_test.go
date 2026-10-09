package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ONSdigital/dis-search-algorithm/algorithm"
	"github.com/ONSdigital/dis-search-algorithm/testset/stream"
	fileStoreMocks "github.com/ONSdigital/dis-search-algorithm/testset/stream/mocks"
	dpEsClient "github.com/ONSdigital/dp-elasticsearch/v4/client"
	dpEsClientMock "github.com/ONSdigital/dp-elasticsearch/v4/client/mocks"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

func TestEvaluateTermsFullStore(t *testing.T) {
	Convey("Given two documents, one term, and one stored judgement", t, func() {
		documents := []stream.Item{
			{Name: docNameCPI, Body: []byte(`{"title":"CPI","uri":"/cpi"}`)},
			{Name: docNameGrowth, Body: []byte(`{"title":"Growth","uri":"/growth"}`)},
		}
		terms := []stream.Item{
			{Name: termIDCPI, Body: []byte(`{"id":"cpi-latest","term":"cpi"}`)},
		}
		judgements := map[string]stream.Item{
			termIDCPI: {
				Name: termIDCPI,
				Body: []byte(`{"term_id":"cpi-latest","judgements":[{"doc_id":"cpi-latest","relevance":4}]}`),
			},
		}
		fakeDocumentStore := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(ctx context.Context) ([]stream.Item, error) {
				return documents, nil
			},
		}
		fakeTermsStore := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(ctx context.Context) ([]stream.Item, error) {
				return terms, nil
			},
		}
		fakeJudgementStore := &fileStoreMocks.StreamMock[stream.Item]{GetFunc: func(ctx context.Context, id string) (stream.Item, error) {
			item, ok := judgements[id]
			if !ok {
				return stream.Item{}, errors.New("not found")
			}
			return item, nil
		}}

		app := &App{
			Documents:  fakeDocumentStore,
			Terms:      fakeTermsStore,
			Judgements: fakeJudgementStore,
		}
		mockClient := &dpEsClientMock.ClientMock{
			CountIndicesFunc: func(context.Context, []string) ([]byte, error) {
				return []byte(`{"count":2}`), nil
			},
			MultiSearchFunc: func(context.Context, []dpEsClient.Search, *dpEsClient.QueryParams) ([]byte, error) {
				return []byte(`{"responses":[{"hits":{"hits":[{"_id":"cpi-latest"},{"_id":"growth-dataset"}]}}]}`), nil
			},
		}

		Convey("When all terms are evaluated with the baseline algorithm", func() {
			evaluations, err := app.evaluateTerms(context.Background(), mockClient,
				[]algorithm.SearchAlgorithm{algorithm.SearchAlgorithmBaseline})

			Convey("Then the query should request the full document store", func() {
				So(err, ShouldBeNil)
				calls := mockClient.MultiSearchCalls()
				So(calls, ShouldHaveLength, 1)
				So(calls[0].Searches, ShouldNotBeEmpty)

				var query map[string]any
				So(json.Unmarshal(calls[0].Searches[0].Query, &query), ShouldBeNil)
				So(query["size"], ShouldEqual, float64(len(documents)))
			})

			Convey("Then the evaluation should be labelled with the baseline algorithm", func() {
				So(err, ShouldBeNil)
				So(evaluations, ShouldHaveLength, 1)
				So(evaluations[0].Algorithm, ShouldEqual, algorithm.SearchAlgorithmBaseline)
			})

			Convey("Then every returned item should include rank, metadata, and judgement state", func() {
				So(err, ShouldBeNil)
				So(evaluations, ShouldHaveLength, 1)
				So(evaluations[0].Hits, ShouldResemble, []evaluatedHit{
					{
						DocumentID: docNameCPI,
						Rank:       1,
						Relevance:  4,
						Judged:     true,
						Title:      testShortCPITitle,
						URI:        testShortCPIURI,
					},
					{
						DocumentID: docNameGrowth,
						Rank:       2,
						Relevance:  0,
						Judged:     false,
						Title:      testGrowthTitle,
						URI:        testShortGrowthURI,
					},
				})
			})
		})
	})
}

func TestEvaluateTermsMultipleAlgorithms(t *testing.T) {
	Convey("Given two documents and two terms", t, func() {
		documents := []stream.Item{
			{Name: docNameCPI, Body: []byte(`{"title":"CPI","uri":"/cpi"}`)},
			{Name: docNameGrowth, Body: []byte(`{"title":"Growth","uri":"/growth"}`)},
		}
		terms := []stream.Item{
			{Name: termIDCPI, Body: []byte(`{"id":"cpi-latest","term":"cpi"}`)},
			{Name: termIDGrowth, Body: []byte(`{"id":"growth-dataset","term":"growth"}`)},
		}
		judgements := map[string]stream.Item{
			termIDCPI: {
				Name: termIDCPI,
				Body: []byte(`{"term_id":"cpi-latest","judgements":[{"doc_id":"cpi-latest","relevance":4}]}`),
			},
			termIDGrowth: {
				Name: termIDGrowth,
				Body: []byte(`{"term_id":"growth-dataset","judgements":[{"doc_id":"growth-dataset","relevance":3}]}`),
			},
		}

		fakeDocumentStore := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(ctx context.Context) ([]stream.Item, error) {
				return documents, nil
			},
		}
		fakeTermsStore := &fileStoreMocks.StreamMock[stream.Item]{
			ListFunc: func(ctx context.Context) ([]stream.Item, error) {
				return terms, nil
			},
		}
		fakeJudgementStore := &fileStoreMocks.StreamMock[stream.Item]{GetFunc: func(ctx context.Context, id string) (stream.Item, error) {
			item, ok := judgements[id]
			if !ok {
				return stream.Item{}, errors.New("not found")
			}
			return item, nil
		}}

		app := &App{
			Documents:  fakeDocumentStore,
			Terms:      fakeTermsStore,
			Judgements: fakeJudgementStore,
		}
		mockClient := &dpEsClientMock.ClientMock{
			CountIndicesFunc: func(context.Context, []string) ([]byte, error) {
				return []byte(`{"count":2}`), nil
			},
			MultiSearchFunc: func(context.Context, []dpEsClient.Search, *dpEsClient.QueryParams) ([]byte, error) {
				return []byte(`{"responses":[{"hits":{"hits":[{"_id":"cpi-latest"},{"_id":"growth-dataset"}]}}]}`), nil
			},
		}
		algorithms := []algorithm.SearchAlgorithm{
			algorithm.SearchAlgorithmBaseline,
			algorithm.SearchAlgorithmUnweighted,
		}

		Convey("When every term is evaluated with both algorithms", func() {
			evaluations, err := app.evaluateTerms(context.Background(), mockClient, algorithms)

			Convey("Then each term should be searched once per algorithm", func() {
				So(err, ShouldBeNil)
				So(mockClient.MultiSearchCalls(), ShouldHaveLength, len(algorithms)*len(terms))
			})

			Convey("Then the index should only be polled once for the whole run", func() {
				So(err, ShouldBeNil)
				So(mockClient.CountIndicesCalls(), ShouldHaveLength, 1)
			})

			Convey("Then the results should be algorithm-major and labelled with their algorithm", func() {
				So(err, ShouldBeNil)
				So(evaluations, ShouldHaveLength, 4)

				labels := make([][2]string, 0, len(evaluations))
				for _, evaluation := range evaluations {
					labels = append(labels, [2]string{string(evaluation.Algorithm), evaluation.Term.ID})
				}
				baseline := string(algorithm.SearchAlgorithmBaseline)
				unweighted := string(algorithm.SearchAlgorithmUnweighted)
				So(labels, ShouldResemble, [][2]string{
					{baseline, docNameCPI},
					{baseline, docNameGrowth},
					{unweighted, docNameCPI},
					{unweighted, docNameGrowth},
				})
			})

			Convey("Then each algorithm should send its own query rather than the baseline query", func() {
				So(err, ShouldBeNil)
				calls := mockClient.MultiSearchCalls()

				// Only the baseline algorithm applies function_score boosts, so
				// its presence proves no silent fallback to baseline occurred.
				weighted := make([]bool, 0, len(calls))
				for _, call := range calls {
					weighted = append(weighted, strings.Contains(string(call.Searches[0].Query), "function_score"))
				}
				So(weighted, ShouldResemble, []bool{true, true, false, false})
			})
		})
	})

	Convey("Given no algorithms to evaluate", t, func() {
		app := &App{
			Documents:  &fileStoreMocks.StreamMock[stream.Item]{},
			Terms:      &fileStoreMocks.StreamMock[stream.Item]{},
			Judgements: &fileStoreMocks.StreamMock[stream.Item]{},
		}

		Convey("When the terms are evaluated", func() {
			evaluations, err := app.evaluateTerms(context.Background(), &dpEsClientMock.ClientMock{}, nil)

			Convey("Then it should reject the empty selection", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldEqual, "at least one search algorithm is required")
				So(evaluations, ShouldBeNil)
			})
		})
	})
}
