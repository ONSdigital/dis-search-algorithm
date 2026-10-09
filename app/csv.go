package app

import (
	"encoding/csv"
	"io"
	"os"

	"github.com/ONSdigital/dis-search-algorithm/ui"
	"github.com/pkg/errors"
)

// CSVColumn represents a single column in a
// CSV file, including its header and
// function to extract its value from a record.
type CSVColumn[T any] struct {
	Header string
	Value  func(T) string
	Set    func(*T, string) error
}

// CSVer defines the interface for reading and writing CSV files.
// This interface allows for mocking CSV read and write operations in tests.
//
//go:generate moq -out mocks/csv.go -pkg mocks . CSVer
type CSVer interface {
	WriteCSV(path string, records []any) error
	ReadCSV(path string) ([]any, error)
}

// CSV represents a generic CSV structure with
// columns and their corresponding value extraction functions.
type CSV[T any] struct {
	Columns  []CSVColumn[T]
	DataType string
}

// NewCSV creates a new CSV instance
// with the given columns.
func NewCSV[T any](columns []CSVColumn[T], dataType string) *CSV[T] {
	return &CSV[T]{
		Columns:  columns,
		DataType: dataType,
	}
}

// WriteCSV writes the given records to a CSV file at the specified path.
// It creates the file if it does not exist and overwrites it if it does.
func (c *CSV[T]) WriteCSV(path string, records []T) error {
	file, err := c.createFile(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return c.writeCSVData(file, records)
}

// ReadCSV reads the CSV file at the specified
// path and returns the records.
// It validates the CSV structure and maps the
// data to the corresponding record type.
func (c *CSV[T]) ReadCSV(path string) ([]T, error) {
	rawRecords, err := c.readFile(path)
	if err != nil {
		return nil, err
	}

	err = c.validateStructure(rawRecords)
	if err != nil {
		return nil, err
	}

	records := make([]T, len(rawRecords)-1) // Exclude header row
	headers := rawRecords[0]

	for i, rawRecord := range rawRecords[1:] {
		var record T
		for j, column := range c.Columns {
			if err := column.Set(&record, rawRecord[j]); err != nil {
				return nil, errors.Wrapf(err, "failed to set value for column %s", headers[j])
			}
		}
		records[i] = record
	}

	ui.Success("imported %s from %s",
		ui.Pluralise(c.DataType, len(records)), path)

	return records, nil
}

func (c *CSV[T]) validateStructure(rawRecords [][]string) error {
	if len(rawRecords) == 0 {
		return errors.New("CSV file is empty")
	}
	headers := rawRecords[0]
	if len(headers) != len(c.Columns) {
		return errors.New("CSV header does not match expected columns")
	}
	for i, column := range c.Columns {
		if headers[i] != column.Header {
			return errors.Errorf("CSV header mismatch at column %d: expected %s, got %s", i, column.Header, headers[i])
		}
	}
	return nil
}

func (c *CSV[T]) readFile(path string) ([][]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open CSV file")
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read CSV file")
	}
	return records, nil
}

func (c *CSV[T]) createFile(path string) (*os.File, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create export file")
	}
	return file, nil
}

func (c *CSV[T]) writeCSVData(w io.Writer, records []T) error {
	writer := csv.NewWriter(w)

	headers := make([]string, len(c.Columns))
	for i, column := range c.Columns {
		headers[i] = column.Header
	}
	if err := writer.Write(headers); err != nil {
		return errors.Wrap(err, "failed to write CSV header")
	}

	for _, record := range records {
		row := make([]string, len(c.Columns))
		for i, column := range c.Columns {
			row[i] = column.Value(record)
		}
		if err := writer.Write(row); err != nil {
			return errors.Wrap(err, "failed to write CSV row")
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return errors.Wrap(err, "failed to flush CSV")
	}

	rowsString := "row"
	if len(records) != 1 {
		rowsString = "rows"
	}

	ui.Success("exported %d %s %s to %s",
		len(records), c.DataType, rowsString, w.(*os.File).Name())

	return nil
}
