package cmd

import (
	"github.com/ONSdigital/dis-search-algorithm/ui"
	"github.com/spf13/cobra"
)

// Load initializes the root command and its subcommands
func Load() (*cobra.Command, error) {
	root := &cobra.Command{
		Use:   "search-testbed",
		Short: "Search relevance testing tool",
		Long: `A comprehensive tool for testing and comparing search algorithm
relevance across different configurations and datasets.`,
		SilenceUsage: true,
	}

	// Add persistent flag for verbose
	root.PersistentFlags().BoolVarP(&ui.Verbose, "verbose", "v", false, "Enable verbose output")

	subCommands, err := getSubCommands()
	if err != nil {
		return nil, err
	}

	root.AddCommand(subCommands...)
	return root, nil
}

func getSubCommands() ([]*cobra.Command, error) {
	exportCmd, err := exportCommand()
	if err != nil {
		return nil, err
	}

	importCmd, err := importCommand()
	if err != nil {
		return nil, err
	}

	return []*cobra.Command{
		compareCommand(),
		exportCmd,
		importCmd,
	}, nil
}
