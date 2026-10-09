package testset

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFixturesFS(t *testing.T) {
	cases := []struct {
		name string
		fsys fs.FS
		dir  string
	}{
		{
			name: "document",
			fsys: DocumentFixturesFS(),
			dir:  DocumentFixturesDir,
		},
		{
			name: "judgement",
			fsys: JudgementFixturesFS(),
			dir:  JudgementFixturesDir,
		},
		{
			name: "term",
			fsys: TermFixturesFS(),
			dir:  TermFixturesDir,
		},
	}

	for _, tc := range cases {
		Convey(fmt.Sprintf("Given the embedded %s fixtures", tc.name), t, func() {
			Convey(fmt.Sprintf("When the %s fixture directory is traversed", tc.name), func() {
				var files []string
				walkErr := fs.WalkDir(tc.fsys, tc.dir, func(filePath string, entry fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if !entry.IsDir() {
						files = append(files, filePath)
					}
					return nil
				})

				Convey("Then it should contain non-empty, valid JSON fixture files", func() {
					So(walkErr, ShouldBeNil)
					So(files, ShouldNotBeEmpty)
					if walkErr != nil {
						return
					}
					for _, filePath := range files {
						data, err := fs.ReadFile(tc.fsys, filePath)
						So(err, ShouldBeNil)
						So(data, ShouldNotBeEmpty)
						So(json.Valid(data), ShouldBeTrue)
					}
				})
			})
		})
	}
}
