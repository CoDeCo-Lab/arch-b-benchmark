package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"benchmark_runner/questionRunner"
	"benchmark_runner/types"

	"github.com/urfave/cli/v3"
)

func main() {
	app := &cli.Command{
		Name:        "benchmark_runner",
		Usage:       "Run and score the released ARCH-B benchmark",
		Description: "Loads benchmark JSON, acquires and verifies images, reproduces prompt construction, queries supported models, and writes scored CSV results.",
		Commands: []*cli.Command{{
			Name:  "run",
			Usage: "Run benchmark questions through one or more models",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "input", Aliases: []string{"i"}, Usage: "Path to released benchmark JSON", Required: true},
				&cli.StringFlag{Name: "mode", Aliases: []string{"m"}, Usage: "answer-only, answer-and-reason, or both", Value: "answer-and-reason"},
				&cli.StringFlag{Name: "slice", Aliases: []string{"s"}, Usage: "Question slice, for example :6, 5:10, or 10:"},
				&cli.StringSliceFlag{Name: "models", Usage: "Provider and model in provider:model format; repeat for multiple models"},
				&cli.BoolFlag{Name: "preflight-only", Usage: "Validate image downloads and SHA-256 hashes without querying a model"},
			},
			Action: runBenchmark,
		}},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runBenchmark(_ context.Context, cmd *cli.Command) error {
	inputFile := cmd.String("input")
	mode := cmd.String("mode")
	models := cmd.StringSlice("models")

	if mode != "answer-only" && mode != "answer-and-reason" && mode != "both" {
		return fmt.Errorf("invalid mode %q: use answer-only, answer-and-reason, or both", mode)
	}

	fmt.Printf("Loading questions from %s...\n", inputFile)
	questions, err := loadQuestions(inputFile, cmd.String("slice"))
	if err != nil {
		return err
	}
	fmt.Printf("Loaded %d questions\n", len(questions))
	if cmd.Bool("preflight-only") {
		if err := questionRunner.PreflightImages(questions); err != nil {
			return err
		}
		fmt.Println("Image preflight passed")
		return nil
	}
	if len(models) == 0 {
		return fmt.Errorf("at least one --models value is required unless --preflight-only is used")
	}

	perModelResults := make(map[string][]types.BenchmarkResult, len(models))
	for _, model := range models {
		runner := questionRunner.NewBenchmarkRunner(model)
		started := time.Now()
		results, err := runner.RunBenchmarkWithMode(questions, mode)
		if err != nil {
			return fmt.Errorf("run %s: %w", model, err)
		}
		runner.PrintSummary(results)
		fmt.Printf("Total execution time for %s: %v\n", model, time.Since(started))
		perModelResults[model] = results

		outputFile := fmt.Sprintf("results_%s_%s.csv", safeModelName(model), inputStem(inputFile))
		if err := questionRunner.WriteCSVResults(results, outputFile); err != nil {
			return err
		}
		if err := runner.WriteSummaryCSV(results, outputFile); err != nil {
			return err
		}
		fmt.Printf("Wrote %s\n", outputFile)
	}

	aggregated := types.AggregateResultsIntoMultiBenchmarkResult(perModelResults)
	outputFile := fmt.Sprintf("results_MULTIBENCHMARK_%s.csv", inputStem(inputFile))
	if err := questionRunner.WriteMultiBenchmarkCSVResults(aggregated, outputFile); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", outputFile)
	return nil
}

func safeModelName(model string) string {
	return strings.NewReplacer(":", "_", "/", "_").Replace(model)
}

func inputStem(inputFile string) string {
	base := filepath.Base(inputFile)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
