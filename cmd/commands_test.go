package cmd

import (
	"io"
	"testing"

	"github.com/ONSdigital/dis-search-test-bed/algorithm"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/cobra"
)

func findCommand(commands []*cobra.Command, name string) *cobra.Command {
	for _, command := range commands {
		if command.Name() == name {
			return command
		}
	}
	return nil
}

func TestLoad(t *testing.T) {
	Convey("Given the loaded root command", t, func() {
		root, err := Load()
		So(err, ShouldBeNil)

		compareCmd := findCommand(root.Commands(), compareCommandName)
		exportCmd := findCommand(root.Commands(), exportCommandName)
		importCmd := findCommand(root.Commands(), importCommandName)

		Convey("Then compare, export and import are registered as sibling top-level commands", func() {
			So(compareCmd, ShouldNotBeNil)
			So(exportCmd, ShouldNotBeNil)
			So(importCmd, ShouldNotBeNil)
		})

		Convey("Then export is not nested as a subcommand of compare", func() {
			So(compareCmd.Commands(), ShouldBeEmpty)
		})

		Convey("Then the parent commands define flags inherited by judgements", func() {
			So(exportCmd.PersistentFlags().Lookup("output"), ShouldNotBeNil)
			So(findCommand(exportCmd.Commands(), "judgements").InheritedFlags().Lookup("output"), ShouldNotBeNil)
			So(importCmd.PersistentFlags().Lookup("input"), ShouldNotBeNil)
			So(findCommand(importCmd.Commands(), "judgements").InheritedFlags().Lookup("input"), ShouldNotBeNil)
		})

		Convey("Then compare accepts an optional list of algorithms", func() {
			flag := compareCmd.Flag(compareAlgorithmFlag)
			So(flag, ShouldNotBeNil)
			So(flag.Shorthand, ShouldEqual, "a")
			So(flag.Value.Type(), ShouldEqual, "stringSlice")
			So(flag.DefValue, ShouldEqual, "[]")
		})

		Convey("Then compare lists the available algorithms in its help", func() {
			for _, name := range algorithm.SearchAlgorithmNames() {
				So(compareCmd.Long, ShouldContainSubstring, name)
			}
		})

		Convey("Then the verbose persistent flag is defined", func() {
			So(root.PersistentFlags().Lookup("verbose"), ShouldNotBeNil)
		})
	})
}

func TestExportCommandRequiresOutput(t *testing.T) {
	Convey("Given export judgements without an output path", t, func() {
		command, err := exportCommand()
		So(err, ShouldBeNil)
		command.SetOut(io.Discard)
		command.SetErr(io.Discard)
		command.SetArgs([]string{"judgements"})

		Convey("When the command is executed", func() {
			err := command.Execute()

			Convey("Then it should reject the missing required flag", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, `required flag(s) "output" not set`)
			})
		})
	})
}

func TestImportCommandRequiresInput(t *testing.T) {
	Convey("Given import judgements without an input path", t, func() {
		command, err := importCommand()
		So(err, ShouldBeNil)
		command.SetOut(io.Discard)
		command.SetErr(io.Discard)
		command.SetArgs([]string{"judgements"})

		Convey("When the command is executed", func() {
			err := command.Execute()

			Convey("Then it should reject the missing required flag", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, `required flag(s) "input" not set`)
			})
		})
	})
}

func TestCompareCommandRejectsUnknownAlgorithm(t *testing.T) {
	Convey("Given a compare command asked for an unregistered algorithm", t, func() {
		command := compareCommand()
		command.SetOut(io.Discard)
		command.SetErr(io.Discard)
		command.SetArgs([]string{"--" + compareAlgorithmFlag + "=baseline,freshness"})

		Convey("When the command is executed", func() {
			// The name is resolved before any Elasticsearch work, so this must
			// fail without starting a container.
			err := command.Execute()

			Convey("Then it should reject the name and list the valid algorithms", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldEqual,
					`unknown algorithm "freshness": valid algorithms are baseline, unweighted`)
			})
		})
	})
}
