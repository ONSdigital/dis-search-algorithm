package app

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ONSdigital/dis-search-algorithm/algorithm"
	"github.com/ONSdigital/dis-search-algorithm/ui"
	dpEsClient "github.com/ONSdigital/dp-elasticsearch/v4/client"
	"github.com/pkg/errors"
)

const (
	csvColumnDocumentID  = "doc_id"
	csvColumnDocumentUri = "uri"
)

// Document represents a single document in the store.
type Document struct {
	ID    string `json:"id"`
	URI   string `json:"uri"`
	Title string `json:"title"`
}

// GetDocument retrieves a document by its ID from the store.
func (a *App) GetDocument(ctx context.Context, documentID string) (Document, error) {
	var document Document
	// Implementation to retrieve a document by its ID from the store
	rawDocument, err := a.Documents.Get(ctx, documentID)
	if err != nil {
		return Document{}, err
	}
	err = json.Unmarshal(rawDocument.Body, &document)
	if err != nil {
		return Document{}, err
	}
	return document, nil
}

// GetDocuments retrieves all existing documents from the store.
func (a *App) GetDocuments(ctx context.Context) ([]Document, error) {
	items, err := a.Documents.List(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list documents")
	}
	existing := make([]Document, len(items))
	for i, item := range items {
		var d Document
		if err := json.Unmarshal(item.Body, &d); err != nil {
			return nil, errors.Wrapf(err, "failed to parse document %q", item.Name)
		}
		existing[i] = d
	}
	return existing, nil
}

// GetDocumentsForTerm retrieves all documents matching a given term.
// TODO: implement a methods whereby we don't create elasticsearch every time.
func (a *App) GetDocumentsForTerm(ctx context.Context, term Term) ([]Document, error) {
	var docs []Document

	err := a.withElasticsearch(ctx, func(ctx context.Context, esClient dpEsClient.Client) error {
		var err error
		docs, err = a.getMatchingDocumentsForTerm(ctx, esClient, term)
		return err
	})
	if err != nil {
		return nil, err
	}
	return docs, nil
}

// ExportDocuments exports the current documents to a
// CSV file at the specified output path.
func (a *App) ExportDocuments(ctx context.Context, outputPath string) error {
	if strings.TrimSpace(outputPath) == "" {
		return errors.New("output path is required")
	}

	documents, err := a.GetDocuments(ctx)
	if err != nil {
		return err
	}

	ui.Info("loaded %s from the store", ui.Pluralise("document", len(documents)))

	codec := createDocumentsCSVCodec()
	err = codec.WriteCSV(outputPath, documents)
	if err != nil {
		return errors.Wrap(err, "failed to write documents to CSV")
	}
	return nil
}

func (a *App) getMatchingDocumentsForTerm(ctx context.Context, esClient dpEsClient.Client, term Term) ([]Document, error) {
	algo := algorithm.SearchAlgorithmBaseline
	registry := algorithm.NewRequestRegistry([]algorithm.SearchAlgorithm{algo})
	builder, err := registry.BuilderFor(algo)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get request builder for algorithm %q", algo)
	}

	searches, err := builder.BuildRequest(ctx, &algorithm.SearchParameters{
		Term:  term.Value,
		Index: indexNameDocuments,
		From:  0,
		Size:  100, // default corpus size, replace with actual value if needed
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build search request for term %q with algorithm %q", term.ID, algo)
	}

	raw, err := esClient.MultiSearch(ctx, searches, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to query term %q with algorithm %q", term.ID, algo)
	}

	rankedIDs, err := parseRankedIDs(raw)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse response for term %q with algorithm %q", term.ID, algo)
	}

	ui.Info("retrieved %d documents for term %q", len(rankedIDs), term.ID)

	var docsForTerm []Document

	for _, id := range rankedIDs {
		document, err := a.GetDocument(ctx, id)
		if err != nil {
			return nil, err
		}
		docsForTerm = append(docsForTerm, document)
	}

	return docsForTerm, nil
}

func createDocumentsCSVCodec() *CSV[Document] {
	return NewCSV([]CSVColumn[Document]{
		{
			Header: csvColumnDocumentID,
			Value:  func(d Document) string { return d.ID },
			Set: func(d *Document, value string) error {
				d.ID = value
				return nil
			},
		},
		{
			Header: csvColumnDocumentUri,
			Value:  func(d Document) string { return d.URI },
			Set: func(d *Document, value string) error {
				d.URI = value
				return nil
			},
		},
	}, "document")
}
