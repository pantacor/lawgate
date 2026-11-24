package lawsdoc

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var ownerDecision = regexp.MustCompile(`(?i)(?:^|[.;]\s*)owner decision (\d{4}-\d{2}-\d{2})\b`)
var agreedCitation = regexp.MustCompile(`(?i)^(.+?)\s+\(agreed (\d{4}-\d{2}-\d{2})\)`)
var sourceURL = regexp.MustCompile(`https?://[^\s<>()\x60]+\b`)
var sourcePath = regexp.MustCompile("\x60([^\x60]+)\x60|[^\\s\x60<>()\\[\\],;*]+")

func checkSource(source string, in Inputs) error {
	if source == "" {
		return fmt.Errorf("has no \"- Source:\" line")
	}
	decisions := ownerDecision.FindAllStringSubmatch(source, -1)
	if err := checkOwnerDecisionDates(decisions); err != nil {
		return err
	}
	if match := agreedCitation.FindStringSubmatch(source); len(match) != 0 {
		citation := hasSourceURL(match[1]) || hasSourcePath(match[1], in)
		if validDate(match[2]) && citation {
			return nil
		}
		return invalidSource(source)
	}
	citation := len(decisions) != 0 || hasSourceURL(source)
	if citation || hasSourcePath(source, in) {
		return nil
	}
	return invalidSource(source)
}

func invalidSource(source string) error {
	return fmt.Errorf("has invalid Source %q: must cite another repository file (excluding go.mod), a URL, or a dated owner decision (YYYY-MM-DD)", source)
}

func checkOwnerDecisionDates(decisions [][]string) error {
	for _, match := range decisions {
		if !validDate(match[1]) {
			return fmt.Errorf("has invalid Source: owner decision date %q is not a valid calendar date", match[1])
		}
	}
	return nil
}

func validDate(date string) bool {
	_, err := time.Parse(time.DateOnly, date)
	return err == nil
}

func hasSourceURL(source string) bool {
	for _, match := range sourceURL.FindAllString(source, -1) {
		parsed, err := url.Parse(match)
		if err == nil && parsed.Hostname() != "" {
			return true
		}
	}
	return false
}

func hasSourcePath(source string, in Inputs) bool {
	if in.RepoRoot == "" {
		return false
	}
	docPath := in.DocPath
	if docPath == "" {
		docPath = FileName
	}
	self, _ := documentFile(in.RepoRoot, "", docPath)
	for _, match := range sourcePath.FindAllStringSubmatch(source, -1) {
		name := sourceFilename(match[0], match[1])
		if allowedSourcePath(in.RepoRoot, docPath, self, name) {
			return true
		}
	}
	return false
}

func allowedSourcePath(root, docPath, self, name string) bool {
	if filepath.Base(name) == "go.mod" {
		return false
	}
	resolved, err := documentFile(root, docPath, name)
	otherFile := resolved != self && filepath.Base(resolved) != "go.mod"
	return err == nil && otherFile
}

func sourceFilename(token, quoted string) string {
	ref := quoted
	if ref == "" && strings.ContainsAny(token, "/.") {
		ref = token
	}
	name, _, _ := strings.Cut(strings.TrimRight(ref, "."), "#")
	return name
}
