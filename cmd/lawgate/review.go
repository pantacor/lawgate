package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/pantacor/lawgate/gosource"
	"github.com/pantacor/lawgate/review"
	"github.com/pantacor/lawgate/review/lsp"
)

// reviewOptions are the flags of `lawgate review`.
type reviewOptions struct {
	root     string
	base     string
	json     bool
	index    bool
	blobURL  string
	requests bool
	lsp      bool
}

func parseReviewOptions(arguments []string) (reviewOptions, error) {
	var options reviewOptions
	flags := flag.NewFlagSet("review", flag.ContinueOnError)
	flags.StringVar(&options.root, "root", "", "repository root or a directory below it (default: the current directory); a directory below the root limits the report to it")
	flags.StringVar(&options.base, "base", "", "the branch or commit the changes are reviewed against (default: the remote's default branch, else origin/main)")
	flags.BoolVar(&options.json, "json", false, "print the report as JSON instead of Markdown")
	flags.BoolVar(&options.index, "index", false, "print the repository's review index as JSON")
	flags.StringVar(&options.blobURL, "blob-url", "", "link files in the Markdown report under this URL (for example https://github.com/org/repo/blob/<sha>)")
	flags.BoolVar(&options.requests, "pr", false, "ask the platform's CLI (gh or glab) which pull or merge request brought each commit")
	flags.BoolVar(&options.lsp, "lsp", false, "serve the language server on standard input and output")
	// LSP clients (vscode-languageclient among them) add --stdio, and may add
	// their process id: both mean -lsp here.
	stdio := flags.Bool("stdio", false, "the same as -lsp")
	flags.String("clientProcessId", "", "ignored: LSP clients pass it")
	if err := flags.Parse(arguments); err != nil {
		return options, err
	}
	if flags.NArg() != 0 {
		return options, errors.New("review accepts flags only")
	}
	options.lsp = options.lsp || *stdio
	return options, nil
}

func reviewCommand(arguments []string, output io.Writer) error {
	options, err := parseReviewOptions(arguments)
	if err != nil {
		return err
	}
	if options.lsp {
		return serveReview(options)
	}
	start := options.root
	if start == "" {
		start = "."
	}
	root, err := review.FindRoot(start)
	if err != nil {
		return fmt.Errorf("no lawgate.json at or above %s", start)
	}
	idx, err := review.Build(root, reviewBuildOptions())
	if err != nil {
		return err
	}
	if options.index {
		return writeJSON(output, idx)
	}
	return printReview(idx, start, options, output)
}

func printReview(idx *review.Index, start string, options reviewOptions, output io.Writer) error {
	base := options.base
	if base == "" {
		base = review.DefaultBase(idx.Root)
	}
	reportOptions := review.ReportOptions{Paths: scope(idx.Root, start), MergeRequests: options.requests}
	report, err := review.Review(idx, base, reportOptions)
	if err != nil {
		return err
	}
	if options.json {
		return writeJSON(output, report)
	}
	_, err = io.WriteString(output, review.Markdown(report, options.blobURL))
	return err
}

// scope limits a review started below the repository root to that
// directory.
func scope(root, start string) []string {
	absolute, err := gosource.CanonicalRoot(start)
	if err != nil {
		return nil
	}
	relative, err := filepath.Rel(root, absolute)
	below := err == nil && relative != "."
	if !below || !filepath.IsLocal(relative) {
		return nil
	}
	return []string{filepath.ToSlash(relative)}
}

// serveReview runs the language server. Without -root it finds the
// repository from the client's workspace folders.
func serveReview(options reviewOptions) error {
	config := lsp.Config{Build: func(root string) (*review.Index, error) {
		return review.Build(root, reviewBuildOptions())
	}}
	if options.root != "" {
		root, err := review.FindRoot(options.root)
		if err != nil {
			return fmt.Errorf("no lawgate.json at or above %s", options.root)
		}
		config.Root = root
	}
	return lsp.Serve(config, os.Stdin, os.Stdout)
}

// reviewBuildOptions run the machine commands through the check's own
// runner, so the review refuses what the check refuses (a symlinked input).
func reviewBuildOptions() review.Options {
	run := func(root, module string, args []string, timeout time.Duration) ([]byte, string, error) {
		return execute(root, module, args, timeout)
	}
	return review.Options{Execute: run}
}

func writeJSON(output io.Writer, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
