package app

import (
	"context"
	"encoding/json"
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
