package app

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/ONSdigital/dis-search-algorithm/algorithm"
	"github.com/ONSdigital/dis-search-algorithm/testset/stream"
	"github.com/ONSdigital/dis-search-algorithm/ui"
	dpEsClient "github.com/ONSdigital/dp-elasticsearch/v4/client"
	"github.com/pkg/errors"
)

const (
	// minRelevance and maxRelevance bound a valid graded relevance judgement.
	minRelevance = 0
	maxRelevance = 4

	csvColumnQueryID          = "query_id"
	csvColumnQuery            = "query"
	csvColumnDocumentID       = "doc_id"
	csvColumnCurrentRelevance = "current_relevance"
	csvColumnTitle            = "title"
	csvColumnURI              = "uri"
)

// relevanceRow is one re-scored grade parsed from an export-format CSV: the
// relevance of a document for a query. The query, title and uri columns are
// carried by the CSV for the reviewer but ignored on import.
type relevanceRow struct {
	QueryID   string
	DocID     string
	Relevance int
}

// importSummary counts the outcome of an import, for logging.
type importSummary struct {
	Applied int // rows whose document is in the corpus
	Skipped int // rows skipped because their doc_id is not in the corpus
	Updated int // judgement files whose contents changed
}

// ImportJudgements reads re-scored relevance judgements
// from the export-format CSV at inputPath and merges them
// back into the judgement store.
// Rows are merged into each query's existing judgements (upsert); a 0 grade
// is treated as unjudged (its entry is removed);
// and rows whose doc_id is not in the document corpus are skipped with a
// warning, so no dangling references are written. Import does not use
// Elasticsearch.
func (a *App) ImportJudgements(ctx context.Context, inputPath string) (err error) {
	if strings.TrimSpace(inputPath) == "" {
		return errors.New("input path is required")
	}
	ui.Info("importing judgements from %s", inputPath)

	file, err := os.Open(inputPath) // #nosec G304 -- operator-supplied path by design
	if err != nil {
		return errors.Wrap(err, "failed to open import file")
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = errors.Wrap(closeErr, "failed to close import file")
		}
	}()

	return a.importCSV(ctx, file)
}

// ExportJudgements evaluates every test term and writes the
// ranked results and their current relevance judgements as
// CSV to outputPath.
func (a *App) ExportJudgements(ctx context.Context, outputPath string) error {
	if strings.TrimSpace(outputPath) == "" {
		return errors.New("output path is required")
	}

	return a.withElasticsearch(ctx, func(ctx context.Context, esClient dpEsClient.Client) error {
		evaluations, err := a.evaluateTerms(ctx, esClient, []algorithm.SearchAlgorithm{exportAlgorithm})
		if err != nil {
			return err
		}
		return writeEvaluationFile(outputPath, evaluations)
	})
}

// importCSV is the io.Reader core of Import (mirrors writeEvaluationsCSV). It
// is split out so the merge can be tested without a file on disk.
func (a *App) importCSV(ctx context.Context, r io.Reader) error {
	rows, err := readRelevanceCSV(r)
	if err != nil {
		return err
	}
	ui.Info("read %d relevance row(s) from CSV", len(rows))

	knownDocIDs, err := a.knownDocumentIDs(ctx)
	if err != nil {
		return err
	}
	ui.Info("loaded %d document(s) from the corpus", len(knownDocIDs))

	existing, err := a.existingJudgements(ctx)
	if err != nil {
		return err
	}
	ui.Info("loaded %d existing judgement file(s)", len(existing))

	updated, summary := buildJudgements(rows, knownDocIDs, existing)
	if summary.Skipped > 0 {
		ui.Warning("skipped %d row(s) referencing documents not in the corpus", summary.Skipped)
	}

	for queryID, entries := range updated {
		if equalEntries(existing[queryID], entries) {
			ui.Debug("judgement %q unchanged, skipping", queryID)
			continue // unchanged: don't rewrite, so a no-op round-trip stays a no-op
		}
		if err := a.putJudgement(ctx, queryID, entries); err != nil {
			return err
		}
		ui.Info("updated judgement %q (%d entries)", queryID, len(entries))
		summary.Updated++
	}

	ui.Success("imported %d relevance row(s); updated %d judgement file(s); skipped %d",
		summary.Applied, summary.Updated, summary.Skipped)
	return nil
}

// readRelevanceCSV parses re-scored relevance rows from an export-format
// CSV. Columns are located by header name (query_id, doc_id,
// current_relevance), so order and extra columns are tolerated. A missing
// required column, a non-integer or out-of-range grade, or an empty
// query_id/doc_id is an error.
func readRelevanceCSV(r io.Reader) ([]relevanceRow, error) {
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
	queryIdx, err := columnIndex(header, csvColumnQueryID)
	if err != nil {
		return nil, err
	}
	docIdx, err := columnIndex(header, csvColumnDocumentID)
	if err != nil {
		return nil, err
	}
	relIdx, err := columnIndex(header, csvColumnCurrentRelevance)
	if err != nil {
		return nil, err
	}
	widest := queryIdx
	for _, idx := range []int{docIdx, relIdx} {
		if idx > widest {
			widest = idx
		}
	}

	rows := make([]relevanceRow, 0, len(records)-1)
	for i, record := range records[1:] {
		line := i + 2 // 1-based line number, past the header
		if len(record) <= widest {
			return nil, errors.Errorf("row %d: expected at least %d columns, got %d", line, widest+1, len(record))
		}

		queryID := strings.TrimSpace(record[queryIdx])
		docID := strings.TrimSpace(record[docIdx])
		if queryID == "" || docID == "" {
			return nil, errors.Errorf("row %d: %s and %s must not be empty", line, csvColumnQueryID, csvColumnDocumentID)
		}

		relevance, err := strconv.Atoi(strings.TrimSpace(record[relIdx]))
		if err != nil {
			return nil, errors.Wrapf(err, "row %d: invalid %s %q", line, csvColumnCurrentRelevance, record[relIdx])
		}
		if relevance < minRelevance || relevance > maxRelevance {
			return nil, errors.Errorf("row %d: %s %d out of range [%d,%d]",
				line, csvColumnCurrentRelevance, relevance, minRelevance, maxRelevance)
		}

		rows = append(rows, relevanceRow{QueryID: queryID, DocID: docID, Relevance: relevance})
	}
	return rows, nil
}

// columnIndex returns the position of the named column in the CSV header.
func columnIndex(header []string, name string) (int, error) {
	for i, column := range header {
		if strings.TrimSpace(column) == name {
			return i, nil
		}
	}
	return 0, errors.Errorf("missing required column %q in CSV header", name)
}

// buildJudgements applies the parsed rows to the existing judgements. It
// returns the updated entries keyed by query_id plus a summary; rows whose
// doc_id is not in knownDocIDs are skipped and counted. It does no I/O, so
// the merge is unit-testable without files or stores.
func buildJudgements(
	rows []relevanceRow,
	knownDocIDs map[string]struct{},
	existing map[string][]judgementEntry,
) (map[string][]judgementEntry, importSummary) {
	var summary importSummary

	rowsByQuery := make(map[string][]relevanceRow)
	for _, row := range rows {
		if _, ok := knownDocIDs[row.DocID]; !ok {
			summary.Skipped++
			continue
		}
		summary.Applied++
		rowsByQuery[row.QueryID] = append(rowsByQuery[row.QueryID], row)
	}

	updated := make(map[string][]judgementEntry, len(rowsByQuery))
	for queryID, queryRows := range rowsByQuery {
		updated[queryID] = mergeJudgement(existing[queryID], queryRows)
	}
	return updated, summary
}

// mergeJudgement upserts one query's rows onto its existing entries: a grade
// above 0 sets or adds the document's relevance, a grade of 0 removes it
// (0 == unjudged, keeping the answer key sparse), and documents the rows
// do not mention are preserved. The result is a non-nil slice sorted by
// doc_id. When the same document appears more than once, the last row wins.
func mergeJudgement(existing []judgementEntry, rows []relevanceRow) []judgementEntry {
	relevanceByDoc := make(map[string]int, len(existing)+len(rows))
	for _, entry := range existing {
		relevanceByDoc[entry.DocID] = entry.Relevance
	}
	for _, row := range rows {
		if row.Relevance == minRelevance {
			delete(relevanceByDoc, row.DocID)
			continue
		}
		relevanceByDoc[row.DocID] = row.Relevance
	}

	entries := make([]judgementEntry, 0, len(relevanceByDoc))
	for docID, relevance := range relevanceByDoc {
		entries = append(entries, judgementEntry{DocID: docID, Relevance: relevance})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].DocID < entries[j].DocID })
	return entries
}

// equalEntries reports whether the updated entries match the existing answer
// key (order-independent), so an unchanged judgement is not rewritten. The
// updated slice is already sorted by doc_id (see mergeJudgement).
func equalEntries(existing, updated []judgementEntry) bool {
	if len(existing) != len(updated) {
		return false
	}
	sorted := make([]judgementEntry, len(existing))
	copy(sorted, existing)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].DocID < sorted[j].DocID })
	for i := range sorted {
		if sorted[i] != updated[i] {
			return false
		}
	}
	return true
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

// existingJudgements loads the current answer keys keyed by query_id. A
// query absent from the store is missing from the map (treated as new).
func (a *App) existingJudgements(ctx context.Context) (map[string][]judgementEntry, error) {
	items, err := a.Judgements.List(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list judgements")
	}
	existing := make(map[string][]judgementEntry, len(items))
	for _, item := range items {
		var j judgement
		if err := json.Unmarshal(item.Body, &j); err != nil {
			return nil, errors.Wrapf(err, "failed to parse judgement %q", item.Name)
		}
		existing[item.Name] = j.Judgements
	}
	return existing, nil
}

// putJudgement writes the query's answer key, keeping the in-body query_id
// equal to the filename.
func (a *App) putJudgement(ctx context.Context, queryID string, entries []judgementEntry) error {
	body, err := marshalJudgement(judgement{QueryID: queryID, Judgements: entries})
	if err != nil {
		return errors.Wrapf(err, "failed to encode judgement %q", queryID)
	}
	if err := a.Judgements.Put(ctx, queryID, stream.Item{Name: queryID, Body: body}); err != nil {
		return errors.Wrapf(err, "failed to write judgement %q", queryID)
	}
	return nil
}

// marshalJudgement renders a judgement in the fixture style:
// 2-space indentation, one inline entry object per line, and a trailing
// newline.
func marshalJudgement(j judgement) ([]byte, error) {
	queryID, err := json.Marshal(j.QueryID)
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "{\n  \"query_id\": %s,\n  \"judgements\": [", queryID)
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

func writeEvaluationFile(path string, evaluations []termEvaluation) error {
	ui.Info("writing results to %s", path)

	file, err := os.Create(path)
	if err != nil {
		return errors.Wrap(err, "failed to create export file")
	}

	writeErr := writeEvaluationsCSV(file, evaluations)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return errors.Wrap(closeErr, "failed to close export file")
	}

	ui.Success("exported %d result row(s) for %d term(s) to %s",
		totalHits(evaluations), len(evaluations), path)
	return nil
}

// totalHits counts the ranked result rows across all evaluations (one CSV row
// per hit).
func totalHits(evaluations []termEvaluation) int {
	total := 0
	for _, evaluation := range evaluations {
		total += len(evaluation.Hits)
	}
	return total
}

func writeEvaluationsCSV(writer io.Writer, evaluations []termEvaluation) error {
	csvWriter := csv.NewWriter(writer)
	if err := csvWriter.Write([]string{
		csvColumnQueryID,
		csvColumnQuery,
		csvColumnDocumentID,
		csvColumnCurrentRelevance,
		csvColumnTitle,
		csvColumnURI,
	}); err != nil {
		return errors.Wrap(err, "failed to write CSV header")
	}

	for _, evaluation := range evaluations {
		for _, hit := range evaluation.Hits {
			if err := csvWriter.Write([]string{
				evaluation.Term.ID,
				evaluation.Term.Query,
				hit.DocumentID,
				strconv.Itoa(hit.Relevance),
				hit.Title,
				hit.URI,
			}); err != nil {
				return errors.Wrap(err, "failed to write CSV row")
			}
		}
	}

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return errors.Wrap(err, "failed to flush CSV")
	}
	return nil
}
