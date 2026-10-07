// Package app runs search-relevance evaluations against a throwaway
// Elasticsearch instance: it indexes the test-set documents, evaluates every
// term with one or more search algorithms, and either logs the scores with a
// comparison of each algorithm's NDCG (Compare) or writes the ranked results
// to a CSV file (Export).
package app

import (
	"github.com/ONSdigital/dis-search-algorithm/algorithm"
	"github.com/ONSdigital/dis-search-algorithm/testset/stream"
)

// exportAlgorithm is the algorithm whose ranking Export evaluates. The export
// CSV has no algorithm column, so the export stays single-algorithm.
const exportAlgorithm = algorithm.SearchAlgorithmBaseline

// App holds the stores the evaluation operates on.
type App struct {
	Documents  stream.Stream[stream.Item] // indexed into Elasticsearch (see storeTargets)
	Terms      stream.Stream[stream.Item] // evaluation data: query terms (not indexed)
	Judgements stream.Stream[stream.Item] // evaluation data: relevance labels (not indexed)
}

// New wires the App to the embedded fixture stores.
func New() *App {
	return &App{
		Documents:  stream.NewDocumentStore(),
		Terms:      stream.NewTermStore(),
		Judgements: stream.NewJudgementStore(),
	}
}
