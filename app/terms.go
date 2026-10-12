package app

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ONSdigital/dis-search-algorithm/ui"
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
