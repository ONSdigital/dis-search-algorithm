package app

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/ONSdigital/dis-search-algorithm/testset/stream"
	"github.com/ONSdigital/dis-search-algorithm/ui"
	"github.com/pkg/errors"
)

const (
	csvColumnQueryID     = "query_id"
	csvColumnQuery       = "query"
	csvColumnDescription = "description"
)

// ImportTerms reads terms from a CSV file
// and imports them into the store.
func (a *App) ImportTerms(ctx context.Context, inputPath string) (err error) {
	if strings.TrimSpace(inputPath) == "" {
		return errors.New("input path is required")
	}
	ui.Info("importing terms from %s", inputPath)

	file, err := os.Open(inputPath) // #nosec G304 -- operator-supplied path by design
	if err != nil {
		return errors.Wrap(err, "failed to open import file")
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = errors.Wrap(closeErr, "failed to close import file")
		}
	}()

	return a.importTermsCSV(ctx, file)
}

// ExportTerms evaluates every test term and writes their
// metadata to the CSV file at outputPath.
func (a *App) ExportTerms(ctx context.Context, outputPath string) error {
	if strings.TrimSpace(outputPath) == "" {
		return errors.New("output path is required")
	}

	// Get the existing terms
	terms, err := a.GetTerms(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get terms")
	}

	return writeTermFile(outputPath, terms)
}

// GetTerms retrieves all existing terms from the store.
func (a *App) GetTerms(ctx context.Context) ([]term, error) {
	items, err := a.Terms.List(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list terms")
	}
	existing := make([]term, len(items))
	for i, item := range items {
		var t term
		if err := json.Unmarshal(item.Body, &t); err != nil {
			return nil, errors.Wrapf(err, "failed to parse term %q", item.Name)
		}
		existing[i] = t
	}
	return existing, nil
}

// importTermsCSV is the io.Reader core of ImportTerms. It
// is split out so the merge can be tested without a file on disk.
func (a *App) importTermsCSV(ctx context.Context, r io.Reader) error {
	rows, err := readTermsCSV(r)
	if err != nil {
		return err
	}
	ui.Info("read %d term row(s) from CSV", len(rows))

	existing, err := a.GetTerms(ctx)
	if err != nil {
		return err
	}
	ui.Info("loaded %d existing term(s)", len(existing))

	existingByID := make(map[string]term, len(existing))
	for _, current := range existing {
		existingByID[current.ID] = current
	}

	incomingIDs := make(map[string]struct{}, len(rows))
	var created, updated, unchanged, deleted int
	for _, incoming := range rows {
		incomingIDs[incoming.ID] = struct{}{}
		current, ok := existingByID[incoming.ID]
		if ok && current == incoming {
			ui.Debug("term %q unchanged, skipping", incoming.ID)
			unchanged++
			continue
		}
		if err := a.putTerm(ctx, incoming); err != nil {
			return err
		}
		if ok {
			updated++
			ui.Info("updated term %q", incoming.ID)
		} else {
			created++
			ui.Info("created term %q", incoming.ID)
		}
	}

	for _, current := range existing {
		if _, ok := incomingIDs[current.ID]; ok {
			continue
		}
		if err := a.Terms.Delete(ctx, current.ID); err != nil {
			return errors.Wrapf(err, "failed to delete term %q", current.ID)
		}
		deleted++
		ui.Info("deleted term %q", current.ID)
	}

	ui.Success("imported %d term row(s); created %d, updated %d, deleted %d, unchanged %d",
		len(rows), created, updated, deleted, unchanged)
	return nil
}

func (a *App) putTerm(ctx context.Context, t term) error {
	body, err := marshalTerm(t)
	if err != nil {
		return errors.Wrapf(err, "failed to encode term %q", t.ID)
	}
	if err := a.Terms.Put(ctx, t.ID, stream.Item{Name: t.ID, Body: body}); err != nil {
		return errors.Wrapf(err, "failed to write term %q", t.ID)
	}
	return nil
}

// marshalTerm renders a term in the fixture style:
// 2-space indentation, one inline entry object per line, and a trailing
// newline.
// I'm not sure this should be responsible for
// JSON encoding, but it currently is. TODO: move formatting to the store.
func marshalTerm(t term) ([]byte, error) {
	body := fmt.Sprintf(
		`{
  "id": %q,
  "query": %q,
  "description": %q
}
`, t.ID, t.Query, t.Description)

	return []byte(body), nil
}

func writeTermFile(path string, terms []term) error {
	ui.Info("writing results to %s", path)

	file, err := os.Create(path)
	if err != nil {
		return errors.Wrap(err, "failed to create export file")
	}

	writeErr := writeTermsCSV(file, terms)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return errors.Wrap(closeErr, "failed to close export file")
	}

	ui.Success("exported %d result row(s) for %d term(s) to %s",
		len(terms), len(terms), path)
	return nil
}

func writeTermsCSV(writer io.Writer, terms []term) error {
	csvWriter := csv.NewWriter(writer)
	if err := csvWriter.Write([]string{
		csvColumnQueryID,
		csvColumnQuery,
		csvColumnDescription,
	}); err != nil {
		return errors.Wrap(err, "failed to write CSV header")
	}

	for _, t := range terms {
		if err := csvWriter.Write([]string{
			t.ID,
			t.Query,
			t.Description,
		}); err != nil {
			return errors.Wrap(err, "failed to write CSV row")
		}
	}

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return errors.Wrap(err, "failed to flush CSV")
	}
	return nil
}

func readTermsCSV(r io.Reader) ([]term, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1 // rows are bounds-checked against the header below

	records, err := reader.ReadAll()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read CSV")
	}
	if len(records) == 0 {
		return nil, errors.New("CSV is empty: expected a header row")
	}

	header := records[0]
	queryIDindex, err := columnIndex(header, csvColumnQueryID)
	if err != nil {
		return nil, err
	}
	queryIndex, err := columnIndex(header, csvColumnQuery)
	if err != nil {
		return nil, err
	}
	descriptionIndex, err := columnIndex(header, csvColumnDescription)
	if err != nil {
		return nil, err
	}

	widest := slices.Max([]int{queryIDindex, queryIndex, descriptionIndex})

	rows := make([]term, 0, len(records)-1)
	for i, record := range records[1:] {
		line := i + 2 // 1-based line number, past the header
		if len(record) <= widest {
			return nil, errors.Errorf("row %d: expected at least %d columns, got %d", line, widest+1, len(record))
		}

		queryID := strings.TrimSpace(record[queryIDindex])
		query := strings.TrimSpace(record[queryIndex])
		if queryID == "" || query == "" {
			return nil, errors.Errorf("row %d: %s and %s must not be empty", line, csvColumnQueryID, csvColumnQuery)
		}

		description := strings.TrimSpace(record[descriptionIndex])
		if description == "" {
			return nil, errors.Errorf("row %d: %s must not be empty", line, csvColumnDescription)
		}

		rows = append(rows, term{ID: queryID, Query: query, Description: description})
	}
	return rows, nil
}
