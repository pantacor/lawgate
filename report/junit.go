package report

import (
	"encoding/xml"
	"fmt"
	"os"
	"strings"

	"github.com/pantacor/lawgate/machine"
)

// JUnitSuites is a JUnit report: a testsuite per machine, a testcase per law.
// A broken law is a failure, a pending one a skip, and a run that could not
// be checked is a suite with one errored case.
type JUnitSuites struct {
	XMLName  xml.Name     `xml:"testsuites"`
	Name     string       `xml:"name,attr"`
	Tests    int          `xml:"tests,attr"`
	Failures int          `xml:"failures,attr"`
	Errors   int          `xml:"errors,attr"`
	Skipped  int          `xml:"skipped,attr"`
	Suites   []JUnitSuite `xml:"testsuite"`
}

// JUnitSuite is one machine. Package carries the machine's module.
type JUnitSuite struct {
	Name     string      `xml:"name,attr"`
	Package  string      `xml:"package,attr,omitempty"`
	Tests    int         `xml:"tests,attr"`
	Failures int         `xml:"failures,attr"`
	Errors   int         `xml:"errors,attr"`
	Skipped  int         `xml:"skipped,attr"`
	Cases    []JUnitCase `xml:"testcase"`
}

// JUnitCase is one law on one machine.
type JUnitCase struct {
	Name      string    `xml:"name,attr"`
	Classname string    `xml:"classname,attr"`
	File      string    `xml:"file,attr,omitempty"`
	Out       string    `xml:"system-out,omitempty"`
	Failure   *JUnitMsg `xml:"failure,omitempty"`
	Error     *JUnitMsg `xml:"error,omitempty"`
	Skipped   *JUnitMsg `xml:"skipped,omitempty"`
}

// JUnitMsg is why a law failed, errored or was skipped.
type JUnitMsg struct {
	Message string `xml:"message,attr,omitempty"`
	Body    string `xml:",chardata"`
}

const (
	junitPendingPrefix = "pending: "
	infrastructureCase = "infrastructure"
)

// JUnitReport renders the runs as a JUnit report.
func JUnitReport(runs []LawRun) JUnitSuites {
	totals := Sum(runs)
	report := JUnitSuites{Name: "laws", Tests: totals.Checked + totals.Checks + totals.Failed, Failures: totals.Broken, Errors: totals.Failed, Skipped: totals.Pending}
	for _, run := range runs {
		report.Suites = append(report.Suites, junitSuite(run))
	}
	return report
}

func junitSuite(run LawRun) JUnitSuite {
	class := moduleMachine(run, ".")
	if reason := run.failure(); reason != "" {
		why := &JUnitMsg{Message: reason}
		failed := JUnitCase{Name: infrastructureCase, Classname: class, Error: why}
		cases := []JUnitCase{failed}
		return JUnitSuite{Name: run.Machine, Package: run.Module, Tests: 1, Errors: 1, Cases: cases}
	}
	suite := JUnitSuite{Name: run.Machine, Package: run.Module, Tests: len(run.Verdicts), Failures: run.Broken(), Skipped: run.Pending()}
	for _, verdict := range run.Verdicts {
		suite.Cases = append(suite.Cases, junitCase(class, run.Source, verdict))
	}
	return suite
}

func junitCase(class, source string, verdict machine.Verdict) JUnitCase {
	c := JUnitCase{Name: verdict.Law.Name, Classname: class, File: source, Out: verdict.Law.Doc}
	body := strings.Join(verdict.Violations, "\n")
	switch {
	case verdict.Broken():
		c.Failure = &JUnitMsg{Message: fmt.Sprintf("%d violation(s)", len(verdict.Violations)), Body: body}
	case verdict.IsPending():
		c.Skipped = &JUnitMsg{Message: junitPendingPrefix + verdict.Law.Pending, Body: body}
	}
	return c
}

// WriteJUnit stores the runs as a JUnit report.
func WriteJUnit(path string, runs []LawRun) error {
	body, err := xml.MarshalIndent(JUnitReport(runs), "", "  ")
	if err != nil {
		return err
	}
	out := append([]byte(xml.Header), body...)
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

func runFromSuite(suite JUnitSuite) LawRun {
	run := LawRun{Module: suite.Package, Machine: suite.Name}
	for _, c := range suite.Cases {
		if c.Error != nil {
			run.Failure = errorDetail(c.Error)
			continue
		}
		run.Source = c.File
		run.Verdicts = append(run.Verdicts, verdictFromCase(c))
	}
	if run.Failure != "" {
		run.Verdicts = nil
	}
	return run
}

func verdictFromCase(c JUnitCase) machine.Verdict {
	law := machine.Law{Name: c.Name, Doc: c.Out}
	verdict := machine.Verdict{Law: law}
	switch {
	case c.Failure != nil:
		verdict.Violations = junitLines(c.Failure.Body, c.Failure.Message)
	case c.Skipped != nil:
		verdict.Law.Pending = pendingReason(c.Skipped.Message)
		verdict.Violations = junitLines(c.Skipped.Body, c.Skipped.Message)
	}
	return verdict
}

func pendingReason(message string) string {
	reason, marked := strings.CutPrefix(message, junitPendingPrefix)
	if !marked {
		return ""
	}
	return strings.TrimSpace(reason)
}

// junitLines splits a body into lines. An empty body falls back to the
// message, so a broken law or a failed run never reads as clean.
func junitLines(body, message string) []string {
	if strings.TrimSpace(body) == "" {
		return []string{message}
	}
	return strings.Split(body, "\n")
}

func errorDetail(message *JUnitMsg) string {
	detail := strings.TrimSpace(strings.Join([]string{message.Message, message.Body}, "\n"))
	if detail == "" {
		return "JUnit error case gave no detail"
	}
	return detail
}
