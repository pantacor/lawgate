package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type goTestEvent struct {
	Action     string
	Package    string
	Test       string
	Output     string
	ImportPath string
}

type testKey struct {
	Package string
	Name    string
}

type testEvidenceLog struct {
	Verdicts []verdict
	tests    testCompletions
}

type testCompletions struct {
	ran       map[string]bool
	nodeCount map[string]int
}

func (tests testCompletions) require(name string) error {
	if tests.nodeCount[name] > 1 {
		return fmt.Errorf("is enforced by ambiguous Node test %q; use a unique test name", name)
	}
	if !tests.ran[name] {
		return fmt.Errorf("is enforced by test %q, which did not run to completion", name)
	}
	return nil
}

type goTestLog struct {
	testEvidenceLog
	started        map[testKey]string
	failedPackages map[string]bool
}

func readTestEvidence(language string, data []byte) (testEvidenceLog, error) {
	if language == "go" {
		return goEvidence(data)
	}
	return nodeEvidence(data)
}

func goEvidence(data []byte) (testEvidenceLog, error) {
	tests := testCompletions{ran: make(map[string]bool)}
	summary := testEvidenceLog{tests: tests}
	log := goTestLog{testEvidenceLog: summary, started: make(map[testKey]string), failedPackages: make(map[string]bool)}
	scanner := testScanner(data)
	for scanner.Scan() {
		var event goTestEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return log.testEvidenceLog, fmt.Errorf("invalid go test -json output: %w", err)
		}
		log.applyGoEvent(event)
	}
	for key := range log.started {
		log.Verdicts = append(log.Verdicts, verdict{key.Package + "/" + key.Name, infrastructure, "test did not finish"})
	}
	return log.testEvidenceLog, completeEvidence(log.testEvidenceLog, scanner.Err())
}

// goTestText renders a go test -json stream as the text go test prints, or
// nothing when the data is not such a stream.
func goTestText(data []byte) string {
	var text strings.Builder
	scanner := testScanner(data)
	for scanner.Scan() {
		var event goTestEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			return ""
		}
		text.WriteString(event.Output)
	}
	return strings.TrimSpace(text.String())
}

func (log *goTestLog) applyGoEvent(event goTestEvent) {
	if event.Test == "" {
		log.recordPackage(event)
		return
	}
	if event.Package == "" {
		return
	}
	key := testKey{event.Package, event.Test}
	switch event.Action {
	case "run":
		log.started[key] = ""
	case "output":
		if output, exists := log.started[key]; exists {
			log.started[key] = testDetail(output + event.Output)
		}
	case "pass", "fail", "skip":
		if output, exists := log.started[key]; exists {
			log.recordGoTest(key, event.Action, output)
			delete(log.started, key)
		}
	}
}

func testDetail(output string) string {
	const limit = 4096
	if len(output) > limit {
		return output[len(output)-limit:]
	}
	return output
}

func (log *goTestLog) recordPackage(event goTestEvent) {
	switch event.Action {
	case "fail":
		log.recordPackageFailure(event.Package)
	case "build-fail":
		log.Verdicts = append(log.Verdicts, verdict{event.ImportPath, infrastructure, "Go package failed to build"})
	}
}

func (log *goTestLog) recordPackageFailure(pkg string) {
	if log.failedPackages[pkg] {
		log.Verdicts = append(log.Verdicts, verdict{pkg, failed, "Go package tests failed"})
		return
	}
	log.Verdicts = append(log.Verdicts, verdict{pkg, infrastructure, "Go package failed without a completed failing test"})
}

func (log *goTestLog) recordGoTest(key testKey, action, output string) {
	status := skipped
	switch action {
	case "pass":
		status = passed
	case "fail":
		status = failed
		log.failedPackages[key.Package] = true
	}
	qualified := key.Package + "/" + key.Name
	if status != skipped {
		log.tests.ran[qualified] = true
	}
	if status == passed {
		output = ""
	}
	log.Verdicts = append(log.Verdicts, verdict{Name: qualified, Status: status, Detail: strings.TrimSpace(output)})
}

var tapTest = regexp.MustCompile(`^\s*(ok|not ok)\s+[0-9]+\s+-\s+(.*)$`)
var tapDirective = regexp.MustCompile(`(?i)\s+#\s*(SKIP|TODO)\b.*$`)

var tapPlan = regexp.MustCompile(`^1\.\.([0-9]+)(?:\s+.*)?$`)

type tapResults struct {
	evidence testEvidenceLog
	header   bool
	expected int
	count    int
	yaml     tapYAML
}

// tapYAML tracks an optional indented YAML diagnostic block ("---" ... "...")
// immediately following a TAP test point line.
type tapYAML struct {
	afterTest  bool
	testIndent string
	active     bool
	yamlIndent string
	verdict    int
}

func nodeEvidence(data []byte) (testEvidenceLog, error) {
	yaml := tapYAML{verdict: -1}
	results := tapResults{expected: -1, yaml: yaml}
	results.evidence.tests.ran = make(map[string]bool)
	results.evidence.tests.nodeCount = make(map[string]int)
	scanner := testScanner(data)
	for scanner.Scan() {
		results.readLine(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return results.evidence, err
	}
	complete := results.header && results.expected == results.count
	if !complete {
		return results.evidence, fmt.Errorf("incomplete Node TAP report")
	}
	return results.evidence, completeEvidence(results.evidence, nil)
}

func (results *tapResults) readLine(line string) {
	if results.yaml.consume(line, &results.evidence) {
		return
	}
	if line == "TAP version 13" {
		results.header = true
	}
	if plan := tapPlan.FindStringSubmatch(line); plan != nil {
		count, err := strconv.Atoi(plan[1])
		if err == nil {
			results.expected = count
		}
	}
	match := tapTest.FindStringSubmatch(line)
	if match == nil {
		return
	}
	indent := leadingSpace(line)
	if indent == "" {
		results.count++
	}
	trimmed := strings.TrimPrefix(line, indent)
	index := recordNodeTest(match[1], match[2], trimmed, &results.evidence)
	results.yaml.expectAfter(indent, index)
}

func (block *tapYAML) expectAfter(indent string, verdictIndex int) {
	block.afterTest = true
	block.testIndent = indent
	block.verdict = verdictIndex
}

func (block *tapYAML) consume(line string, evidence *testEvidenceLog) bool {
	if block.active {
		return block.consumeActive(line, evidence)
	}
	if !block.afterTest {
		return false
	}
	block.afterTest = false
	indent := leadingSpace(line)
	startsBlock := strings.TrimSpace(line) == "---" && isDeeperIndent(indent, block.testIndent)
	if !startsBlock {
		block.verdict = -1
		return false
	}
	block.active = true
	block.yamlIndent = indent
	block.appendLine(line, evidence)
	return true
}

func (block *tapYAML) consumeActive(line string, evidence *testEvidenceLog) bool {
	unindented := strings.TrimSpace(line) != "" && !strings.HasPrefix(line, block.yamlIndent)
	if unindented {
		block.finish()
		return false
	}
	block.appendLine(line, evidence)
	if strings.TrimRight(line, " \t") == block.yamlIndent+"..." {
		block.finish()
	}
	return true
}

func (block *tapYAML) appendLine(line string, evidence *testEvidenceLog) {
	if block.verdict < 0 {
		return
	}
	trimmed := strings.TrimPrefix(line, block.testIndent)
	detail := evidence.Verdicts[block.verdict].Detail
	evidence.Verdicts[block.verdict].Detail = appendTAPDetail(detail, trimmed)
}

func (block *tapYAML) finish() {
	block.active = false
	block.verdict = -1
}

func leadingSpace(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	return line[:len(line)-len(trimmed)]
}

func isDeeperIndent(indent, base string) bool {
	return len(indent) > len(base) && strings.HasPrefix(indent, base)
}

func appendTAPDetail(existing, line string) string {
	const limit = 4096
	if len(existing) >= limit {
		return existing
	}
	combined := existing + "\n" + line
	if len(combined) > limit {
		return combined[:limit]
	}
	return combined
}

func recordNodeTest(outcome, name, line string, evidence *testEvidenceLog) int {
	status := passed
	if outcome == "not ok" {
		status = failed
	}
	if directive := tapDirective.FindStringIndex(name); directive != nil {
		status = skipped
		name = name[:directive[0]]
	}
	name = strings.ReplaceAll(name, "\\#", "#")
	evidence.tests.nodeCount[name]++
	if status != skipped {
		evidence.tests.ran[name] = true
	}
	detail := ""
	index := -1
	if status == failed {
		detail = line
		index = len(evidence.Verdicts)
	}
	evidence.Verdicts = append(evidence.Verdicts, verdict{Name: name, Status: status, Detail: detail})
	return index
}

func testScanner(data []byte) *bufio.Scanner {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 4<<20)
	return scanner
}

func completeEvidence(evidence testEvidenceLog, err error) error {
	if err != nil {
		return err
	}
	if len(evidence.tests.ran) == 0 && !containsFailure(evidence.Verdicts) {
		return fmt.Errorf("test runner produced zero executed test verdicts")
	}
	return nil
}
