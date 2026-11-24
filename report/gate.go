package report

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteGate writes the run summary, page and one JUnit artifact per module.
func WriteGate(directory string, meta Meta, groups [][]LawRun, notes []string) error {
	var runs []LawRun
	var err error
	for index, group := range groups {
		name := filepath.Join(directory, fmt.Sprintf("%03d.xml", index+1))
		err = errors.Join(err, replaceReport(name, func(temp string) error {
			return WriteJUnit(temp, group)
		}))
		runs = append(runs, group...)
	}
	summary := filepath.Join(directory, "laws-summary.md")
	page := filepath.Join(directory, "laws-report.html")
	return errors.Join(err, replaceReport(summary, func(temp string) error {
		return os.WriteFile(temp, []byte(gateSummary(runs, notes)), 0644)
	}), replaceReport(page, func(temp string) error {
		return WriteHTML(temp, NewHTMLData(runs, meta))
	}))
}

func gateSummary(runs []LawRun, notes []string) string {
	var summary strings.Builder
	summary.WriteString(Markdown(runs))
	if len(notes) > 0 {
		summary.WriteString("\n### Notes\n\n")
	}
	for _, note := range notes {
		fmt.Fprintln(&summary, "- "+note)
	}
	return summary.String()
}

func replaceReport(path string, write func(string) error) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".report-")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Close(); err != nil {
		return err
	}
	if err := write(name); err != nil {
		return err
	}
	return os.Rename(name, path)
}
