package app

import (
	"context"
	"encoding/json"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/ONSdigital/dis-search-algorithm/algorithm"
	"github.com/ONSdigital/dis-search-algorithm/testset/stream"
	"github.com/ONSdigital/dis-search-algorithm/ui"
	dpEsClient "github.com/ONSdigital/dp-elasticsearch/v4/client"
	"github.com/ONSdigital/dp-search-api/api"
	searchModels "github.com/ONSdigital/dp-search-api/models"
	searchSDK "github.com/ONSdigital/dp-search-api/sdk"
	"github.com/pkg/errors"
)

const (
	csvColumnDocumentID    = "doc_id"
	csvColumnDocumentUri   = "uri"
	csvColumnDocumentTitle = "title"
	defaultSearchAPIURL    = "https://api.beta.ons.gov.uk/v1"
)

// Document represents a single document in the store.
type Document struct {
	ID   string            `json:"id"`
	URI  string            `json:"uri"`
	Item searchModels.Item `json:"item"`
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
func (a *App) GetDocumentsForTerm(ctx context.Context, term Term, esClient dpEsClient.Client) ([]Document, error) {
	return a.getMatchingDocumentsForTerm(ctx, esClient, term)
}

// PutDocument stores a document in the store.
// It returns true if the document was newly added,
// or false if it already existed.
func (a *App) PutDocument(ctx context.Context, d Document) (bool, error) {
	existingDocument, err := a.GetDocument(ctx, d.ID)
	if err != nil && !errors.Is(err, stream.ErrItemNotFound) {
		return false, errors.Wrapf(err, "failed to check existing document %q", d.ID)
	}
	if err == nil && reflect.DeepEqual(d, existingDocument) {
		ui.Info("document %q already exists and is unchanged", d.ID)
		return false, nil
	}

	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return false, errors.Wrapf(err, "failed to encode document %q", d.ID)
	}
	data = append(data, '\n')
	if err := a.Documents.Put(ctx, d.ID, stream.Item{Name: d.ID, Body: data}); err != nil {
		return false, errors.Wrapf(err, "failed to write document %q", d.ID)
	}
	return true, nil
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

// ImportDocuments imports documents from a CSV file
// at the specified input path.
func (a *App) ImportDocuments(ctx context.Context, inputPath string) error {
	if strings.TrimSpace(inputPath) == "" {
		return errors.New("input path is required")
	}

	ui.Info("importing documents from the CSV at %q", inputPath)

	codec := createDocumentsCSVCodec()
	documents, err := codec.ReadCSV(inputPath)
	if err != nil {
		return errors.Wrap(err, "failed to read documents from CSV")
	}
	ui.Info("loaded %s from the CSV", ui.Pluralise("document", len(documents)))

	enrichedDocuments, err := a.getDocumentsMetadata(ctx, documents)
	if err != nil {
		return err
	}

	ui.Success("retrieved metadata for %s", ui.Pluralise("document", len(enrichedDocuments)))

	err = a.importDocumentsToStore(ctx, enrichedDocuments, documents)
	if err != nil {
		return err
	}

	err = a.RebuildAllTermJudgements(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (a *App) importDocumentsToStore(ctx context.Context, documents, importedDocuments []Document) error {
	ui.Info("importing %s to store", ui.Pluralise("document", len(documents)))
	var summary importSummary
	existingDocuments, err := a.GetDocuments(ctx)
	if err != nil {
		return err
	}

	for i := range documents {
		d := documents[i]
		updated, err := a.PutDocument(ctx, d)
		if err != nil {
			return errors.Wrapf(err, "failed to import document %q", d.ID)
		}
		if updated {
			summary.Updated++
		} else {
			summary.Skipped++
		}
	}
	removed, err := a.removeDocumentsFromStore(ctx, existingDocuments, importedDocuments)
	if err != nil {
		return err
	}
	summary.Removed = removed

	ui.Info("import summary: %d updated, %d skipped, %d removed", summary.Updated, summary.Skipped, summary.Removed)
	return nil
}

func (a *App) removeDocumentsFromStore(ctx context.Context, existingDocuments, importedDocuments []Document) (int, error) {
	importedIDs := make(map[string]bool, len(importedDocuments))
	for i := range importedDocuments {
		importedIDs[importedDocuments[i].ID] = true
	}

	removed := 0
	for i := range existingDocuments {
		document := existingDocuments[i]
		if importedIDs[document.ID] {
			continue
		}
		if err := a.Documents.Delete(ctx, document.ID); err != nil {
			return removed, errors.Wrapf(err, "failed to remove document %q", document.ID)
		}
		removed++
		ui.Info("document %q removed from store", document.ID)
	}
	return removed, nil
}

func (a *App) getDocumentsMetadata(ctx context.Context, documents []Document) ([]Document, error) {
	var enriched []Document

	//TODO: could make the search API URL configurable
	searchClient := searchSDK.New(defaultSearchAPIURL)
	spinner := ui.NewSpinner("retrieving document metadata")
	spinner.Start()
	defer spinner.Stop()

	// TODO: could make the batch size configurable
	const batchSize = 20
	for start := 0; start < len(documents); start += batchSize {
		end := min(start+batchSize, len(documents))
		batch, err := a.getDocumentMetadataBatch(ctx, documents[start:end], searchClient)
		if err != nil {
			return nil, err
		}
		enriched = append(enriched, batch...)
		if end < len(documents) {
			//TODO: make sleep configurable to avoid hitting rate limits
			time.Sleep(500 * time.Millisecond)
		}
	}
	return enriched, nil
}

func (a *App) getDocumentMetadataBatch(ctx context.Context, docs []Document, searchClient *searchSDK.Client) ([]Document, error) {
	ui.Info("getting metadata for batch of %s", ui.Pluralise("document", len(docs)))

	uris := make([]string, 0, len(docs))
	for i := range docs {
		uris = append(uris, docs[i].URI)
	}

	// Ensure we return all the results for the batch.
	options := searchSDK.Options{
		Query: url.Values{},
	}
	options.Limit(strconv.Itoa(len(uris)))

	response, err := searchClient.PostSearchURIs(ctx, options, api.URIsRequest{URIs: uris})
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve document metadata")
	}
	itemsByURI := make(map[string]searchModels.Item, len(response.Items))
	for i := range response.Items {
		itemsByURI[response.Items[i].URI] = response.Items[i]
	}
	enriched := make([]Document, 0, len(docs))
	for i := range docs {
		doc := docs[i]
		item, ok := itemsByURI[doc.URI]
		if !ok {
			ui.Warning("metadata not returned for document %q (URI %q)", doc.ID, doc.URI)
			continue
		}
		doc.Item = item
		enriched = append(enriched, doc)
	}
	return enriched, nil
}

func (a *App) getMatchingDocumentsForTerm(ctx context.Context, esClient dpEsClient.Client, term Term) ([]Document, error) {
	algo := algorithm.SearchAlgorithmBaseline
	registry := algorithm.NewRequestRegistry([]algorithm.SearchAlgorithm{algo})
	builder, err := registry.BuilderFor(algo)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get request builder for algorithm %q", algo)
	}

	const pageSize = 50
	var docsForTerm []Document
	for from := 0; ; from += pageSize {
		searches, err := builder.BuildRequest(ctx, &algorithm.SearchParameters{
			Term:  term.Value,
			Index: indexNameDocuments,
			From:  from,
			Size:  pageSize,
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

		for _, id := range rankedIDs {
			document, err := a.GetDocument(ctx, id)
			if err != nil {
				return nil, err
			}
			docsForTerm = append(docsForTerm, document)
		}
		if len(rankedIDs) < pageSize {
			break
		}
	}

	ui.Info("retrieved %d documents for term %q", len(docsForTerm), term.ID)
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
		{
			Header: csvColumnDocumentTitle,
			Value:  func(d Document) string { return d.Item.Title },
			Set: func(d *Document, value string) error {
				d.Item.Title = value
				return nil
			},
		},
	}, "document")
}
