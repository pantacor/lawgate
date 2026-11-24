package lawsdoc

import (
	"slices"
	"strings"

	"github.com/pantacor/lawgate/mutate"
)

// readSubject reads the "- Subject:" lines: functions, one or more per line
// separated by commas, or a single "not yet. <why>" gap.
func (plan *LawPlan) readSubject() {
	value, present := plan.Fields[FieldSubject]
	if !present {
		return
	}
	if !slices.ContainsFunc(plan.Enforcers, isTestEnforcer) {
		plan.fail("\"Subject:\" names what a test protects but the law has no test enforcer")
		return
	}
	// A gap mixed with subjects reaches gapReason, which refuses it.
	gap := strings.HasPrefix(value, "not yet") || strings.Contains(value, "\nnot yet")
	if gap {
		plan.SubjectGap = plan.gapReason(FieldSubject, value)
		return
	}
	plan.readSubjects(value)
}

func (plan *LawPlan) readSubjects(value string) {
	if strings.TrimSpace(value) == "" {
		plan.fail("\"Subject:\" names no function: write <import path>.<Func> or \"not yet. <why>\"")
		return
	}
	for reference := range strings.FieldsFuncSeq(value, isSubjectSeparator) {
		subject, err := mutate.ParseSubject(reference)
		if err != nil {
			plan.fail("has an invalid \"Subject:\": %v", err)
			continue
		}
		plan.Subjects = append(plan.Subjects, subject)
	}
}

func isTestEnforcer(enforcer Enforcer) bool {
	return enforcer.Kind == Test
}

func isSubjectSeparator(r rune) bool {
	return r == ',' || r == '\n'
}
