package review

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/lawsdoc"
)

// readDocuments reads the module's LAWS.md and the shared ones it lists.
func (b *builder) readDocuments(module inventory.Module) {
	documents := append([]string{module.Laws}, module.SharedLaws...)
	for _, document := range documents {
		b.idx.addDocument(module.Path, document)
		if b.documents[document] {
			continue
		}
		b.documents[document] = true
		b.readDocument(module.Path, document)
	}
}

func (b *builder) readDocument(module, document string) {
	data, err := os.ReadFile(filepath.Join(b.idx.Root, filepath.FromSlash(document)))
	if err != nil {
		b.problem(Problem{Module: module, File: document, Message: fmt.Sprintf("read %s: %v", document, err)})
		return
	}
	lines := strings.Split(string(data), "\n")
	for _, plan := range lawsdoc.Plan(string(data)) {
		law := documentedLaw(module, document, plan, lines)
		b.idx.addLaw(law)
	}
}

func documentedLaw(module, document string, plan lawsdoc.LawPlan, lines []string) *Law {
	end := sectionEnd(lines, plan.Line)
	return &Law{
		Name:       plan.Name,
		Module:     module,
		File:       document,
		Line:       plan.Line,
		EndLine:    end,
		Text:       strings.TrimSpace(strings.Join(lines[plan.Line-1:end], "\n")),
		Source:     plan.Fields[lawsdoc.FieldSource],
		EnforcedBy: plan.Fields[lawsdoc.FieldEnforcedBy],
		Status:     plan.Fields[lawsdoc.FieldStatus],
		Subject:    plan.Fields[lawsdoc.FieldSubject],
		subjects:   plan.Subjects,
		testNames:  testEnforcers(plan),
	}
}

func testEnforcers(plan lawsdoc.LawPlan) []string {
	var names []string
	for _, enforcer := range plan.Enforcers {
		if enforcer.Kind == lawsdoc.Test {
			names = append(names, enforcer.Name)
		}
	}
	return names
}

var anyHeading = regexp.MustCompile(`^#{1,6} `)

// sectionEnd is the last line of the section whose heading is on line
// heading: the line before the next heading outside a code fence, without
// trailing blank lines.
func sectionEnd(lines []string, heading int) int {
	end := len(lines)
	fenced := false
	for number := heading + 1; number <= len(lines); number++ {
		line := lines[number-1]
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
		}
		if !fenced && anyHeading.MatchString(line) {
			end = number - 1
			break
		}
	}
	for end > heading && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return end
}

// addDocument records that module reads document, for LawFor.
func (idx *Index) addDocument(module, document string) {
	if idx.documents == nil {
		idx.documents = map[string][]string{}
	}
	idx.documents[module] = append(idx.documents[module], document)
}

// addLaw keys the law by its name, or by "<file>#<name>" when another
// module's law already holds the name ("<module>#<name>" for a law only
// code knows).
func (idx *Index) addLaw(law *Law) {
	law.Key = law.Name
	if _, taken := idx.Laws[law.Key]; taken {
		law.Key = cmp.Or(law.File, law.Module) + "#" + law.Name
	}
	idx.Laws[law.Key] = law
}

// moduleLaw finds the law named name that module documents (in its LAWS.md
// or a shared one it lists).
func (idx *Index) moduleLaw(module, name string) *Law {
	for _, document := range idx.documents[module] {
		if law := idx.Laws[name]; law != nil && law.File == document {
			return law
		}
		if law := idx.Laws[document+"#"+name]; law != nil {
			return law
		}
	}
	return nil
}

// LawFor resolves a law name used in file: the law the file's module
// documents, else any law of that name.
func (idx *Index) LawFor(name, file string) *Law {
	if law := idx.moduleLaw(idx.moduleOf(file), name); law != nil {
		return law
	}
	return idx.Laws[name]
}
