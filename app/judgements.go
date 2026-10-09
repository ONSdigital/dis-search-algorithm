package app

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/ONSdigital/dis-search-algorithm/testset/stream"
	"github.com/ONSdigital/dis-search-algorithm/ui"
	"github.com/pkg/errors"
)

const (
	// minRelevance and maxRelevance bound a valid graded relevance judgement.
	minRelevance = 0
	maxRelevance = 4

	csvColumnTermID           = "term_id"
	csvColumnTerm             = "term"
	csvColumnDocumentID       = "doc_id"
	csvColumnCurrentRelevance = "current_relevance"
	csvColumnTitle            = "title"
	csvColumnURI              = "uri"
)

// importSummary counts the outcome of an import, for logging.
type judgementImportSummary struct {
	Skipped int // rows skipped because they are not valid judgements
	Updated int // judgement files whose contents changed
}

// TermJudgements groups all judgements for a single term.
type TermJudgements struct {
	TermID     string      `json:"term_id"`
	Judgements []Judgement `json:"judgements"`
}

// Judgement represents the relevance judgement for a single document.
type Judgement struct {
	DocID     string `json:"doc_id"`
	Relevance int    `json:"relevance"`
}

// TermJudgement is single row for import export that represents a
// judgement for a document for a particular term.
type TermJudgement struct {
	Judgement Judgement
	Document  Document
	Term      Term
}

// ImportJudgements reads re-scored relevance judgements
// from the export-format CSV at inputPath and merges them
// back into the judgement store.
func (a *App) ImportJudgements(ctx context.Context, inputPath string) (err error) {
	if strings.TrimSpace(inputPath) == "" {
		return errors.New("input path is required")
	}
	ui.Info("importing judgements from %s", inputPath)

	codec := createJudgementCSVCodec()

	judgements, err := codec.ReadCSV(inputPath)
	if err != nil {
		return err
	}

	if len(judgements) == 0 {
		return errors.New("no judgements to import")
	}

	return a.importJudgementsToStore(ctx, judgements)
}

// ExportJudgements evaluates every test term and writes the
// ranked results and their current relevance judgements as
// CSV to outputPath.
func (a *App) ExportJudgements(ctx context.Context, outputPath string) error {
	if strings.TrimSpace(outputPath) == "" {
		return errors.New("output path is required")
	}

	judgements, err := a.Judgements.List(ctx)
	if err != nil {
		return err
	}

	var rows []TermJudgement

	ui.Info("loaded %s from the store", ui.Pluralise("judgement file", len(judgements)))

	for _, judgement := range judgements {
		var termJudgements TermJudgements
		err = json.Unmarshal(judgement.Body, &termJudgements)
		if err != nil {
			return err
		}

		judgementRows, err := a.buildTermJudgementRows(ctx, termJudgements)
		if err != nil {
			return err
		}
		rows = append(rows, judgementRows...)
	}

	codec := createJudgementCSVCodec()
	codec.WriteCSV(outputPath, rows)
	return nil
}

// importCSV is the io.Reader core of Import (mirrors writeEvaluationsCSV). It
// is split out so the merge can be tested without a file on disk.
func (a *App) importJudgementsToStore(ctx context.Context, termJudgements []TermJudgement) error {
	var summary judgementImportSummary

	knownDocIDs, err := a.knownDocumentIDs(ctx)
	if err != nil {
		return err
	}
	ui.Info("loaded %s from the store", ui.Pluralise("document", len(knownDocIDs)))

	existingJudgements, err := a.existingJudgements(ctx)
	if err != nil {
		return err
	}

	ui.Info("loaded %s from the store", ui.Pluralise("existing judgement file", len(existingJudgements)))

	var validatedJudgements []TermJudgement

	for _, judgement := range termJudgements {
		valid, err := a.validateTermJudgement(ctx, judgement)
		if err != nil {
			return err
		}
		if valid {
			validatedJudgements = append(validatedJudgements, judgement)
		} else {
			ui.Error("invalid judgement for document %s, term %s, relevance %d - judgement rejected", judgement.Document.ID, judgement.Term.ID, judgement.Judgement.Relevance)
		}
	}

	newJudgements := buildTermJudgements(validatedJudgements)

	ui.Info("built %d new judgements", len(newJudgements))

	for _, judgement := range newJudgements {
		updated, err := a.putJudgement(ctx, judgement)
		if err != nil {
			return err
		}
		if updated {
			ui.Info("updated judgement %q (%d entries)", judgement.TermID, len(judgement.Judgements))
			summary.Updated++
		} else {
			ui.Info("skipped judgement %q (%d entries)", judgement.TermID, len(judgement.Judgements))
			summary.Skipped++
		}
	}

	ui.Info("checking if we should remove any judgements from the store")

	removed, err := a.removeJudgementsFromStore(ctx, existingJudgements, newJudgements)
	if err != nil {
		return err
	}

	ui.Success("judgement file updates: %d updated; %d skipped; %d removed",
		summary.Updated, summary.Skipped, removed)
	return nil
}

func (a *App) removeJudgementsFromStore(ctx context.Context, existingJudgements, newJudgements []TermJudgements) (int, error) {
	count := 0

	for _, judgement := range existingJudgements {
		// Check if this existing judgement is in the new judgements
		index := slices.IndexFunc(newJudgements, func(item TermJudgements) bool {
			return item.TermID == judgement.TermID
		})
		if index == -1 {
			err := a.deleteJudgement(ctx, judgement.TermID)
			if err != nil {
				ui.Error("failed to delete existing judgement %q: %v", judgement.TermID, err)
				return count, err
			} else {
				count++
				ui.Info("judgement removed from store for term %s", judgement.TermID)
			}
		}
	}
	return count, nil
}

// buildTermJudgements converts rows of TermJudgement
// into composite TermJudgements
func buildTermJudgements(judgements []TermJudgement) []TermJudgements {
	var composite []TermJudgements

	for _, judgement := range judgements {
		// Check if we have one already by index
		index := slices.IndexFunc(composite, func(item TermJudgements) bool {
			return item.TermID == judgement.Term.ID
		})

		judgementEntry := judgement.Judgement

		if index == -1 {
			composite = append(composite, TermJudgements{
				TermID:     judgement.Term.ID,
				Judgements: []Judgement{judgementEntry},
			})
		} else {
			existing := composite[index]
			existing.Judgements = append(existing.Judgements, judgementEntry)
			composite[index] = existing
		}
	}
	return composite
}

func (a *App) buildTermJudgementRows(ctx context.Context, termJudgement TermJudgements) ([]TermJudgement, error) {
	rows := make([]TermJudgement, len(termJudgement.Judgements))

	for i, j := range termJudgement.Judgements {
		term, err := a.GetTerm(ctx, termJudgement.TermID)
		if err != nil {
			return nil, err
		}

		document, err := a.GetDocument(ctx, j.DocID)
		if err != nil {
			return nil, err
		}

		rows[i] = TermJudgement{
			Term:      term,
			Judgement: j,
			Document:  document,
		}
	}
	return rows, nil
}

func (a *App) validateTermJudgement(ctx context.Context, judgement TermJudgement) (bool, error) {
	knownDocumentIDs, err := a.knownDocumentIDs(ctx)
	if err != nil {
		return false, err
	}
	_, exists := knownDocumentIDs[judgement.Document.ID]
	if !exists {
		return false, nil
	}

	if judgement.Judgement.Relevance < minRelevance || judgement.Judgement.Relevance > maxRelevance {
		return false, nil
	}

	// TODO: This should validate the term too.

	return true, nil
}

// knownDocumentIDs returns the set of document ids in the corpus, used to
// reject judgements that reference a document not in the data set.
func (a *App) knownDocumentIDs(ctx context.Context) (map[string]struct{}, error) {
	documents, err := a.Documents.List(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list documents")
	}
	known := make(map[string]struct{}, len(documents))
	for _, item := range documents {
		known[item.Name] = struct{}{}
	}
	return known, nil
}

// existingJudgements loads the current judgements
func (a *App) existingJudgements(ctx context.Context) ([]TermJudgements, error) {
	items, err := a.Judgements.List(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list judgements")
	}
	existing := make([]TermJudgements, len(items))
	for i, item := range items {
		var j TermJudgements
		if err := json.Unmarshal(item.Body, &j); err != nil {
			return nil, errors.Wrapf(err, "failed to parse judgement %q", item.Name)
		}
		existing[i] = j
	}
	return existing, nil
}

// putJudgement writes the term's answer key, keeping the in-body term_id
// equal to the filename.
func (a *App) putJudgement(ctx context.Context, termJudgement TermJudgements) (bool, error) {
	rawExistingJudgement, _ := a.Judgements.Get(ctx, termJudgement.TermID)
	// deliberately not handling the error here

	if rawExistingJudgement.Body != nil {
		var existingJudgement TermJudgements

		if err := json.Unmarshal(rawExistingJudgement.Body, &existingJudgement); err != nil {
			return false, errors.Wrapf(err, "failed to parse existing judgement for %q", termJudgement.TermID)
		}

		if slices.Equal(existingJudgement.Judgements, termJudgement.Judgements) {
			return false, nil
		}
	}

	body, err := marshalJudgement(termJudgement)
	if err != nil {
		return false, errors.Wrapf(err, "failed to encode judgement %q", termJudgement.TermID)
	}
	if err := a.Judgements.Put(ctx, termJudgement.TermID, stream.Item{Name: termJudgement.TermID, Body: body}); err != nil {
		return false, errors.Wrapf(err, "failed to write judgement %q", termJudgement.TermID)
	}
	return true, nil
}

// putJudgement writes the term's answer key, keeping the in-body term_id
// equal to the filename.
func (a *App) deleteJudgement(ctx context.Context, termID string) error {
	return a.Judgements.Delete(ctx, termID)
}

// marshalJudgement renders a judgement in the fixture style:
// 2-space indentation, one inline entry object per line, and a trailing
// newline.
func marshalJudgement(j TermJudgements) ([]byte, error) {
	termID, err := json.Marshal(j.TermID)
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "{\n  \"term_id\": %s,\n  \"judgements\": [", termID)
	if len(j.Judgements) == 0 {
		b.WriteString("]\n}\n")
		return []byte(b.String()), nil
	}

	b.WriteString("\n")
	for i, entry := range j.Judgements {
		docID, err := json.Marshal(entry.DocID)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "    { \"doc_id\": %s, \"relevance\": %d }", docID, entry.Relevance)
		if i < len(j.Judgements)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("  ]\n}\n")
	return []byte(b.String()), nil
}

func createJudgementCSVCodec() *CSV[TermJudgement] {
	return NewCSV([]CSVColumn[TermJudgement]{
		{
			Header: csvColumnTermID,
			Value:  func(t TermJudgement) string { return t.Term.ID },
			Set: func(t *TermJudgement, value string) error {
				t.Term.ID = value
				return nil
			},
		},
		{
			Header: csvColumnTerm,
			Value:  func(t TermJudgement) string { return t.Term.Value },
			Set: func(t *TermJudgement, value string) error {
				t.Term.Value = value
				return nil
			},
		},
		{
			Header: csvColumnDocumentID,
			Value:  func(t TermJudgement) string { return t.Document.ID },
			Set: func(t *TermJudgement, value string) error {
				t.Document.ID = value
				t.Judgement.DocID = value
				return nil
			},
		},
		{
			Header: csvColumnCurrentRelevance,
			Value:  func(t TermJudgement) string { return strconv.Itoa(t.Judgement.Relevance) },
			Set: func(t *TermJudgement, value string) error {
				v, err := strconv.Atoi(value)
				if err != nil {
					return errors.Wrap(err, "failed to convert relevance to int")
				}
				t.Judgement.Relevance = v
				return nil
			},
		},
		{
			Header: csvColumnTitle,
			Value:  func(t TermJudgement) string { return t.Document.Title },
			Set: func(t *TermJudgement, value string) error {
				t.Document.Title = value
				return nil
			},
		},
		{
			Header: csvColumnURI,
			Value:  func(t TermJudgement) string { return t.Document.URI },
			Set: func(t *TermJudgement, value string) error {
				t.Document.URI = value
				return nil
			},
		},
	}, "judgement")
}
