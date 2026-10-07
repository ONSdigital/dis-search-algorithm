package main

import (
	"context"
	"os"

	"github.com/ONSdigital/dis-search-algorithm/cmd"
	"github.com/ONSdigital/dis-search-algorithm/ui"
)

func main() {
	if err := run(); err != nil {
		ui.Error("error on search relevancy tool execution: %s", err.Error())
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	rootCommand, err := cmd.Load()
	if err != nil {
		return err
	}

	return rootCommand.ExecuteContext(ctx)
}
