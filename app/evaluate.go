package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ONSdigital/dis-search-algorithm/algorithm"
	"github.com/ONSdigital/dis-search-algorithm/scoring"
	"github.com/ONSdigital/dis-search-algorithm/testset/stream"
	"github.com/ONSdigital/dis-search-algorithm/ui"
	dpEsClient "github.com/ONSdigital/dp-elasticsearch/v4/client"
	"github.com/pkg/errors"
)

const (
	// searchableTimeout bounds how long we wait for freshly indexed documents
	// to become searchable (Elasticsearch is near-real-time).
	searchableTimeout  = 5 * time.Second
	searchablePollWait = 250 * time.Millisecond
)

// term is the decoded body of a testset/terms fixture.
type term struct {
	ID    string `json:"id"`
	Query string `json:"query"`
}

// judgement is the decoded body of a testset/judgements fixture: the relevance
// answer key for a single term.
type judgement struct {
	QueryID    string           `json:"query_id"`
	Judgements []judgementEntry `json:"judgements"`
}

type judgementEntry struct {
	DocID     string `json:"doc_id"`
	Relevance int    `json:"relevance"`
}

type documentMetadata struct {
	Title string `json:"title"`
	URI   string `json:"uri"`
}

type evaluatedHit struct {
	DocumentID string
	Rank       int
	Relevance  int
	Judged     bool
	Title      string
	URI        string
}

type termEvaluation struct {
	Algorithm algorithm.SearchAlgorithm
	Term      Term
	Hits      []evaluatedHit
	DCG       float64
	IDCG      float64
	NDCG      float64
}

// evaluationContext is the per-run data shared by every term and algorithm. It
// is loaded once, so evaluating another algorithm costs only its searches and
// not a second pass over the fixtures or another wait for the index.
type evaluationContext struct {
	documentByID    map[string]documentMetadata
	corpusSize      int
	terms           []Term
	relevanceByTerm map[string]map[string]int // term id -> document id -> relevance
}

// msearchResponse is the subset of an Elasticsearch _msearch response we need:
// the ranked document ids of each sub-search.
type msearchResponse struct {
	Responses []struct {
		Hits struct {
			Hits []struct {
				ID string `json:"_id"`
			} `json:"hits"`
		} `json:"hits"`
	} `json:"responses"`
}

// countResponse is the subset of an Elasticsearch _count response we need.
type countResponse struct {
	Count int `json:"count"`
}

// evaluateTerms queries Elasticsearch for every test term with each of the
// given algorithms, then returns and logs their full-corpus relevance
// evaluations. Results are algorithm-major: every term of the first algorithm,
// then every term of the second, and so on.
func (a *App) evaluateTerms(ctx context.Context, esClient dpEsClient.Client,
	algorithms []algorithm.SearchAlgorithm) ([]termEvaluation, error) {
	if len(algorithms) == 0 {
		return nil, errors.New("at least one search algorithm is required")
	}

	// Resolve every builder before the first search, so a missing builder
	// fails the run up front rather than part way through it.
	registry := algorithm.NewRequestRegistry(algorithms)
	builders := make([]algorithm.SearchRequestBuilder, 0, len(algorithms))
	for _, algo := range algorithms {
		builder, err := registry.BuilderFor(algo)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get request builder for algorithm %q", algo)
		}
		builders = append(builders, builder)
	}

	evalCtx, err := a.loadEvaluationContext(ctx, esClient)
	if err != nil {
		return nil, err
	}

	evaluations := make([]termEvaluation, 0, len(algorithms)*len(evalCtx.terms))
	for i, algo := range algorithms {
		for _, t := range evalCtx.terms {
			evaluation, err := evaluateTerm(ctx, esClient, builders[i], algo, t, evalCtx)
			if err != nil {
				return nil, err
			}

			evaluations = append(evaluations, evaluation)

			ui.Info("term %q (%s) [%s]: DCG=%.4f IDCG=%.4f NDCG=%.4f",
				t.Value, t.ID, algo,
				evaluation.DCG, evaluation.IDCG, evaluation.NDCG)
		}
	}

	return evaluations, nil
}

// loadEvaluationContext loads everything an evaluation run needs that does not
// depend on the algorithm: the document corpus and its metadata, the test
// terms, and each term's relevance answer key. It also waits for the indexed
// documents to become searchable.
func (a *App) loadEvaluationContext(ctx context.Context, esClient dpEsClient.Client) (*evaluationContext, error) {
	documents, err := a.Documents.List(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list documents")
	}

	documentByID, err := buildDocumentMetadata(documents)
	if err != nil {
		return nil, err
	}

	if err := a.waitForSearchable(ctx, esClient, indexNameDocuments, len(documents)); err != nil {
		return nil, err
	}

	items, err := a.Terms.List(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list terms")
	}

	terms := make([]Term, 0, len(items))
	relevanceByTerm := make(map[string]map[string]int, len(items))
	for _, item := range items {
		var t Term
		if err := json.Unmarshal(item.Body, &t); err != nil {
			return nil, errors.Wrapf(err, "failed to parse term %q", item.Name)
		}

		relevanceByDoc, err := a.relevanceForTerm(ctx, t.ID)
		if err != nil {
			return nil, err
		}

		terms = append(terms, t)
		relevanceByTerm[t.ID] = relevanceByDoc
	}

	return &evaluationContext{
		documentByID:    documentByID,
		corpusSize:      len(documents),
		terms:           terms,
		relevanceByTerm: relevanceByTerm,
	}, nil
}

// evaluateTerm runs one term through one algorithm and scores the ranking it
// returns against the term's relevance answer key.
func evaluateTerm(ctx context.Context, esClient dpEsClient.Client, builder algorithm.SearchRequestBuilder,
	algo algorithm.SearchAlgorithm, t Term, evalCtx *evaluationContext) (termEvaluation, error) {
	searches, err := builder.BuildRequest(ctx, &algorithm.SearchParameters{
		Term:  t.Value,
		Index: indexNameDocuments,
		From:  0,
		Size:  evalCtx.corpusSize,
	})
	if err != nil {
		return termEvaluation{}, errors.Wrapf(err, "failed to build request for term %q with algorithm %q", t.ID, algo)
	}

	raw, err := esClient.MultiSearch(ctx, searches, nil)
	if err != nil {
		return termEvaluation{}, errors.Wrapf(err, "failed to query term %q with algorithm %q", t.ID, algo)
	}

	rankedIDs, err := parseRankedIDs(raw)
	if err != nil {
		return termEvaluation{}, errors.Wrapf(err, "failed to parse response for term %q with algorithm %q", t.ID, algo)
	}

	relevanceByDoc := evalCtx.relevanceByTerm[t.ID]

	hits := make([]evaluatedHit, 0, len(rankedIDs))
	for rank, id := range rankedIDs {
		document, ok := evalCtx.documentByID[id]
		if !ok {
			return termEvaluation{}, errors.Errorf("search returned document %q which is not in the document store", id)
		}
		relevance, judged := relevanceByDoc[id]
		hits = append(hits, evaluatedHit{
			DocumentID: id,
			Rank:       rank + 1,
			Relevance:  relevance,
			Judged:     judged,
			Title:      document.Title,
			URI:        document.URI,
		})
	}

	dcg, idcg, ndcg := scoreRanking(rankedIDs, relevanceByDoc)

	return termEvaluation{
		Algorithm: algo,
		Term:      t,
		Hits:      hits,
		DCG:       dcg,
		IDCG:      idcg,
		NDCG:      ndcg,
	}, nil
}

func buildDocumentMetadata(documents []stream.Item) (map[string]documentMetadata, error) {
	documentByID := make(map[string]documentMetadata, len(documents))
	for _, item := range documents {
		var document documentMetadata
		if err := json.Unmarshal(item.Body, &document); err != nil {
			return nil, errors.Wrapf(err, "failed to parse document %q", item.Name)
		}
		documentByID[item.Name] = document
	}
	return documentByID, nil
}

// relevanceForTerm returns the term's relevance answer key as a map of document
// id to graded relevance. Judgement filenames match their term_id, so the
// judgement is fetched directly by term id.
func (a *App) relevanceForTerm(ctx context.Context, termID string) (map[string]int, error) {
	item, err := a.Judgements.Get(ctx, termID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get judgement for term %q", termID)
	}

	var j TermJudgements
	if err := json.Unmarshal(item.Body, &j); err != nil {
		return nil, errors.Wrapf(err, "failed to parse judgement %q", termID)
	}

	relevanceByDoc := make(map[string]int, len(j.Judgements))
	for _, entry := range j.Judgements {
		relevanceByDoc[entry.DocID] = entry.Relevance
	}

	return relevanceByDoc, nil
}

// scoreRanking computes DCG, IDCG and NDCG for one term's complete ranked
// results.
// rankedIDs are the returned document ids in rank order; relevanceByDoc is the
// term's answer key (a returned id absent from it is unjudged, relevance 0).
func scoreRanking(rankedIDs []string, relevanceByDoc map[string]int) (dcg, idcg, ndcg float64) {
	actual := make([]int, 0, len(rankedIDs))
	for _, id := range rankedIDs {
		actual = append(actual, relevanceByDoc[id])
	}

	judged := make([]int, 0, len(relevanceByDoc))
	for _, relevance := range relevanceByDoc {
		judged = append(judged, relevance)
	}

	dcg = scoring.CalculateDCG(actual)
	idcg = scoring.CalculateIDCG(judged)
	if idcg > 0 {
		ndcg = dcg / idcg
	}

	return dcg, idcg, ndcg
}

// waitForSearchable blocks until all indexed documents are searchable, or the
// searchableTimeout elapses. Elasticsearch is near-real-time and the client
// exposes no explicit refresh, so we poll the document count.
func (a *App) waitForSearchable(ctx context.Context, esClient dpEsClient.Client, index string, expected int) error {
	deadline := time.Now().Add(searchableTimeout)
	for {
		raw, err := esClient.CountIndices(ctx, []string{index})
		if err != nil {
			return errors.Wrapf(err, "failed to count index %s", index)
		}

		var count countResponse
		if err := json.Unmarshal(raw, &count); err != nil {
			return errors.Wrap(err, "failed to parse count response")
		}

		if count.Count >= expected {
			return nil
		}

		if time.Now().After(deadline) {
			return errors.Errorf("timed out waiting for %d documents to become searchable in index %s (found %d)",
				expected, index, count.Count)
		}

		time.Sleep(searchablePollWait)
	}
}

// parseRankedIDs extracts the ranked document ids from the term search, which
// is the first sub-search of the multisearch response (the remaining
// sub-searches are count aggregations).
func parseRankedIDs(raw []byte) ([]string, error) {
	var resp msearchResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal msearch response")
	}

	if len(resp.Responses) == 0 {
		return nil, nil
	}

	hits := resp.Responses[0].Hits.Hits
	ids := make([]string, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.ID)
	}

	return ids, nil
}
