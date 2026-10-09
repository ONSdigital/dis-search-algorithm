package app

import (
	"context"
	"encoding/json"
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
