package app

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/ONSdigital/dis-search-algorithm/testset/stream"
	"github.com/ONSdigital/dis-search-algorithm/ui"
	dpEsClient "github.com/ONSdigital/dp-elasticsearch/v4/client"
	"github.com/pkg/errors"
)

const (
	csvColumnTermID      = "term_id"
	csvColumnTerm        = "term"
	csvColumnDescription = "description"
)

// Term represents a search term
// with an ID and a value.
type Term struct {
	ID          string `json:"id"`
	Value       string `json:"term"`
	Description string `json:"description"`
}

// GetTerm retrieves a term by its ID from the store.
// It returns the term and any error encountered.
func (a *App) GetTerm(ctx context.Context, termID string) (Term, error) {
	var term Term
	// Implementation to retrieve a term by its ID from the store
	rawTerm, err := a.Terms.Get(ctx, termID)
	if err != nil {
		return Term{}, err
	}
	err = json.Unmarshal(rawTerm.Body, &term)
	if err != nil {
		return Term{}, err
	}
	return term, nil
}

// GetTerms retrieves all existing terms from the store.
func (a *App) GetTerms(ctx context.Context) ([]Term, error) {
	items, err := a.Terms.List(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list terms")
	}
	existing := make([]Term, len(items))
	for i, item := range items {
		var t Term
		if err := json.Unmarshal(item.Body, &t); err != nil {
			return nil, errors.Wrapf(err, "failed to parse term %q", item.Name)
		}
		existing[i] = t
	}
	return existing, nil
}

// PutTerm stores a term in the store if it does not
// already exist or has changed.
// It returns a boolean indicating whether the term
// was updated and any error encountered.
func (a *App) PutTerm(ctx context.Context, t Term) (bool, error) {
	existingTerm, err := a.GetTerm(ctx, t.ID)
	if err != nil && !errors.Is(err, stream.ErrItemNotFound) {
		return false, errors.Wrapf(err, "failed to check existing term %q", t.ID)
	}
	if existingTerm == t {
		// Term already exists, no need to put it again
		return false, nil
	}

	body, err := marshalTerm(t)
	if err != nil {
		return false, errors.Wrapf(err, "failed to encode term %q", t.ID)
	}
	if err := a.Terms.Put(ctx, t.ID, stream.Item{Name: t.ID, Body: body}); err != nil {
		return false, errors.Wrapf(err, "failed to write term %q", t.ID)
	}
	return true, nil
}

// DeleteTerm deletes a term from the store by its ID.
func (a *App) DeleteTerm(ctx context.Context, termID string) error {
	return a.Terms.Delete(ctx, termID)
}

// marshalTerm renders a term in the fixture style:
// 2-space indentation, one inline entry object per line, and a trailing
// newline.
// I'm not sure this should be responsible for
// JSON encoding, but it currently is. TODO: move formatting to the store.
func marshalTerm(t Term) ([]byte, error) {
	body := fmt.Sprintf(
		`{
  "id": %q,
  "term": %q,
  "description": %q
}
`, t.ID, t.Value, t.Description)

	return []byte(body), nil
}

// ExportTerms exports the current terms to a
// CSV file at the specified output path.
func (a *App) ExportTerms(ctx context.Context, outputPath string) error {
	if strings.TrimSpace(outputPath) == "" {
		return errors.New("output path is required")
	}

	terms, err := a.GetTerms(ctx)
	if err != nil {
		return err
	}

	ui.Info("loaded %s from the store", ui.Pluralise("term", len(terms)))

	codec := createTermsCSVCodec()
	err = codec.WriteCSV(outputPath, terms)
	if err != nil {
		return errors.Wrap(err, "failed to write terms to CSV")
	}
	return nil
}

// ImportTerms imports terms from a CSV file at the specified input path.
// It returns any error encountered during the import process.
func (a *App) ImportTerms(ctx context.Context, inputPath string) error {
	if strings.TrimSpace(inputPath) == "" {
		return errors.New("input path is required")
	}

	ui.Info("importing terms from %s", inputPath)

	codec := createTermsCSVCodec()
	terms, err := codec.ReadCSV(inputPath)
	if err != nil {
		return errors.Wrap(err, "failed to read terms from CSV")
	}

	if len(terms) == 0 {
		return errors.New("no terms to import")
	}
	return a.withElasticsearch(ctx, func(ctx context.Context, esClient dpEsClient.Client) error {
		return a.importTermsToStore(ctx, terms, esClient)
	})
}

func (a *App) importTermsToStore(ctx context.Context, terms []Term, esClient dpEsClient.Client) error {
	var termSummary importSummary

	ui.Info("getting existing terms from the store for term")
	existingTerms, err := a.GetTerms(ctx)
	if err != nil {
		return err
	}

	ui.Success("retrieved %s from the store", ui.Pluralise("term", len(existingTerms)))

	for _, term := range terms {
		updated, err := a.PutTerm(ctx, term)
		if err != nil {
			return err
		}
		if updated {
			ui.Info("updated term %q", term.ID)
			termSummary.Updated++
		} else {
			ui.Info("skipped term %q", term.ID)
			termSummary.Skipped++
		}

		ui.Info("checking if term %q has a pre-existing judgement", term.ID)
		_, err = a.GetTermJudgements(ctx, term.ID)
		if err != nil {
			if errors.Is(err, stream.ErrItemNotFound) {
				ui.Info("no pre-existing judgement found for term %q, creating default judgements", term.ID)

				matchingDocs, err := a.GetDocumentsForTerm(ctx, term, esClient)
				if err != nil {
					return err
				}

				judgements := make([]Judgement, len(matchingDocs))
				for i, doc := range matchingDocs {
					judgements[i] = Judgement{
						DocID:     doc.ID,
						Relevance: 0, // default relevance
					}
				}
				ui.Info("%s created for term %q", ui.Pluralise("judgement", len(judgements)), term.ID)

				// If no judgement exists for this term, create default judgements for all matching documents.
				_, err = a.putJudgement(ctx, TermJudgements{Judgements: judgements, TermID: term.ID})
				if err != nil {
					return err
				}
			} else {
				return err
			}
		}
	}

	removed, err := a.removeTermsFromStore(ctx, existingTerms, terms)
	if err != nil {
		return err
	}

	ui.Info("import summary: %d updated, %d skipped, %d removed", termSummary.Updated, termSummary.Skipped, removed)

	return nil
}

func (a *App) removeTermsFromStore(ctx context.Context, existingTerms, newTerms []Term) (int, error) {
	count := 0

	for _, term := range existingTerms {
		// Check if this existing term is in the new terms
		index := slices.IndexFunc(newTerms, func(item Term) bool {
			return item.ID == term.ID
		})
		if index == -1 {
			err := a.DeleteTerm(ctx, term.ID)
			if err != nil {
				ui.Error("failed to delete existing term %q: %v", term.ID, err)
				return count, err
			} else {
				count++
				ui.Info("term removed from store for term %s", term.ID)
			}
			err = a.deleteJudgement(ctx, term.ID)
			if err != nil {
				ui.Error("failed to delete existing judgement for term %q: %v", term.ID, err)
				return count, err
			} else {
				ui.Info("judgement removed from store for term %s", term.ID)
			}
		}
	}
	return count, nil
}

func createTermsCSVCodec() *CSV[Term] {
	return NewCSV([]CSVColumn[Term]{
		{
			Header: csvColumnTermID,
			Value:  func(t Term) string { return t.ID },
			Set: func(t *Term, value string) error {
				t.ID = value
				return nil
			},
		},
		{
			Header: csvColumnTerm,
			Value:  func(t Term) string { return t.Value },
			Set: func(t *Term, value string) error {
				t.Value = value
				return nil
			},
		},
		{
			Header: csvColumnDescription,
			Value:  func(t Term) string { return t.Description },
			Set: func(t *Term, value string) error {
				t.Description = value
				return nil
			},
		},
	}, "term")
}
