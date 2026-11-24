package report

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"
)

type junitCounts struct {
	Tests    *int `xml:"tests,attr"`
	Failures *int `xml:"failures,attr"`
	Errors   *int `xml:"errors,attr"`
	Skipped  *int `xml:"skipped,attr"`
}

type inputSuite struct {
	Name    string `xml:"name,attr"`
	Package string `xml:"package,attr"`
	junitCounts
	Cases []JUnitCase `xml:"testcase"`
}

type inputSuites struct {
	junitCounts
	Suites []inputSuite `xml:"testsuite"`
}

// ReadJUnit reads machine verdicts from a single JUnit suite or a suite collection.
func ReadJUnit(path string) ([]LawRun, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: JUnit report is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	const limit = 16 << 20
	reader := &io.LimitedReader{R: file, N: limit + 1}
	runs, err := decodeJUnit(xml.NewDecoder(reader))
	if reader.N <= 0 {
		return nil, fmt.Errorf("%s: JUnit report exceeds %d bytes", path, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return runs, nil
}

func decodeJUnit(decoder *xml.Decoder) ([]LawRun, error) {
	token, err := nextJUnitToken(decoder)
	if err != nil {
		return nil, err
	}
	root, ok := token.(xml.StartElement)
	if !ok {
		return nil, fmt.Errorf("expected JUnit root element")
	}
	input, err := decodeSuites(decoder, root)
	if err != nil {
		return nil, err
	}
	if _, err := nextJUnitToken(decoder); err != io.EOF {
		return nil, fmt.Errorf("unexpected content after JUnit report")
	}
	return input.runs()
}

func nextJUnitToken(decoder *xml.Decoder) (xml.Token, error) {
	for {
		token, err := decoder.Token()
		if err != nil || !xmlPadding(token) {
			return token, err
		}
	}
}

func xmlPadding(token xml.Token) bool {
	switch value := token.(type) {
	case xml.Comment, xml.ProcInst:
		return true
	case xml.CharData:
		return strings.TrimSpace(string(value)) == ""
	}
	return false
}

func decodeSuites(decoder *xml.Decoder, root xml.StartElement) (inputSuites, error) {
	switch root.Name.Local {
	case "testsuite":
		var suite inputSuite
		err := decoder.DecodeElement(&suite, &root)
		suites := []inputSuite{suite}
		return inputSuites{Suites: suites}, err
	case "testsuites":
		var input inputSuites
		err := decoder.DecodeElement(&input, &root)
		return input, err
	default:
		return inputSuites{}, fmt.Errorf("unexpected JUnit root %q", root.Name.Local)
	}
}

func (input inputSuites) runs() ([]LawRun, error) {
	var runs []LawRun
	var all []JUnitCase
	seen := make(map[string]bool)
	for _, suite := range input.Suites {
		if err := validateSuite(suite, seen); err != nil {
			return nil, fmt.Errorf("JUnit suite %q: %w", suite.Name, err)
		}
		run := runFromSuite(JUnitSuite{Name: suite.Name, Package: suite.Package, Cases: suite.Cases})
		runs = append(runs, run)
		all = append(all, suite.Cases...)
	}
	return runs, input.junitCounts.validate(all)
}

func validateSuite(suite inputSuite, seen map[string]bool) error {
	if strings.TrimSpace(suite.Name) == "" {
		return fmt.Errorf("machine suite has no name")
	}
	key := suite.Package + "\x00" + suite.Name
	if seen[key] {
		return fmt.Errorf("duplicate machine suite")
	}
	seen[key] = true
	cases := make(map[string]bool)
	for _, item := range suite.Cases {
		if err := validateJUnitCase(item, cases); err != nil {
			return err
		}
	}
	return suite.junitCounts.validate(suite.Cases)
}

func validateJUnitCase(item JUnitCase, seen map[string]bool) error {
	if strings.TrimSpace(item.Name) == "" {
		return fmt.Errorf("JUnit verdict has no name")
	}
	if seen[item.Name] {
		return fmt.Errorf("duplicate verdict for law %q", item.Name)
	}
	seen[item.Name] = true
	statuses := 0
	for _, message := range []*JUnitMsg{item.Failure, item.Error, item.Skipped} {
		if message != nil {
			statuses++
		}
	}
	if statuses > 1 {
		return fmt.Errorf("conflicting verdicts for law %q", item.Name)
	}
	if item.Skipped != nil && pendingReason(item.Skipped.Message) == "" {
		return fmt.Errorf("skipped law %q has no declared pending reason", item.Name)
	}
	return nil
}

func (declared junitCounts) validate(cases []JUnitCase) error {
	if len(cases) == 0 {
		return fmt.Errorf("machine report has zero verdicts")
	}
	var failures, errors, skipped int
	for _, item := range cases {
		switch {
		case item.Error != nil:
			errors++
		case item.Failure != nil:
			failures++
		case item.Skipped != nil:
			skipped++
		}
	}
	testsMatch := sameCount(declared.Tests, len(cases))
	failuresMatch := sameCount(declared.Failures, failures)
	errorsMatch := sameCount(declared.Errors, errors)
	skippedMatch := sameCount(declared.Skipped, skipped)
	countsMatch := testsMatch && failuresMatch
	statusesMatch := errorsMatch && skippedMatch
	if !countsMatch || !statusesMatch {
		return fmt.Errorf("JUnit totals do not match its cases")
	}
	return nil
}

func sameCount(declared *int, actual int) bool {
	return declared == nil || *declared == actual
}
