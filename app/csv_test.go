package app

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type csvTestRecord struct {
	Name  string
	Count int
}

func newTestCSV() *CSV[csvTestRecord] {
	return NewCSV([]CSVColumn[csvTestRecord]{
		{
			Header: "name",
			Value:  func(record csvTestRecord) string { return record.Name },
			Set: func(record *csvTestRecord, value string) error {
				record.Name = value
				return nil
			},
		},
		{
			Header: "count",
			Value: func(record csvTestRecord) string {
				return strconv.Itoa(record.Count)
			},
			Set: func(record *csvTestRecord, value string) error {
				count, err := strconv.Atoi(value)
				if err != nil {
					return err
				}
				record.Count = count
				return nil
			},
		},
	}, "record")
}

func TestCSVWriteAndRead(t *testing.T) {
	Convey("Given a CSV codec and records containing special characters", t, func() {
		codec := newTestCSV()
		path := filepath.Join(t.TempDir(), "records.csv")
		want := []csvTestRecord{
			{Name: "alpha, beta", Count: 2},
			{Name: "line\nbreak", Count: 0},
		}

		Convey("When the records are written and read", func() {
			writeErr := codec.WriteCSV(path, want)
			got, readErr := codec.ReadCSV(path)

			Convey("Then all record values should round-trip", func() {
				So(writeErr, ShouldBeNil)
				So(readErr, ShouldBeNil)
				So(got, ShouldResemble, want)
			})
		})
	})
}

func TestCSVReadRejectsHeaderMismatch(t *testing.T) {
	Convey("Given a CSV file with headers in the wrong order", t, func() {
		path := filepath.Join(t.TempDir(), "records.csv")
		writeErr := os.WriteFile(path, []byte("count,name\n2,alpha\n"), 0600)

		Convey("When the CSV is read", func() {
			_, readErr := newTestCSV().ReadCSV(path)

			Convey("Then the header mismatch should be reported", func() {
				So(writeErr, ShouldBeNil)
				So(readErr, ShouldBeError, "CSV header mismatch at column 0: expected name, got count")
			})
		})
	})
}

func TestCSVReadWrapsColumnSetterError(t *testing.T) {
	Convey("Given a CSV row with a count that cannot be parsed", t, func() {
		path := filepath.Join(t.TempDir(), "records.csv")
		writeErr := os.WriteFile(path, []byte("name,count\nalpha,not-a-number\n"), 0600)

		Convey("When the CSV is read", func() {
			_, readErr := newTestCSV().ReadCSV(path)

			Convey("Then the error should identify the column and preserve the cause", func() {
				So(writeErr, ShouldBeNil)
				So(readErr, ShouldNotBeNil)
				So(readErr.Error(), ShouldContainSubstring, "failed to set value for column count")
				So(readErr.Error(), ShouldContainSubstring, "invalid syntax")
			})
		})
	})
}
