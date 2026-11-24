package lawsdoc

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var statusAgreedRE = regexp.MustCompile(`^agreed (\d{4}-\d{2}-\d{2})\b`)

// Inputs is the evidence a LAWS.md is checked against.
type Inputs struct {
	Required       map[string][]string          // machine laws that must have a section here
	Machines       map[string][]string          // machine verdicts, indexed by machine name
	Tests          Tests                        // compiled tests for standalone documentation checks
	SharedMachines []string                     // when set, machine "*" applies to these adopting machines
	CheckTest      func(law, name string) error // active checker, when test execution evidence is available
	RepoRoot       string
	DocPath        string // path of the document relative to RepoRoot
}

// Check checks a LAWS.md against the code and returns one line per problem.
func Check(doc string, in Inputs) []string {
	return CheckPlan(Plan(doc), in)
}

// CheckPlan validates the same plan the gate uses to execute enforcers.
func CheckPlan(plans []LawPlan, in Inputs) []string {
	problems := duplicateSections(plans)
	problems = append(problems, missingSections(plans, in.Required)...)
	for _, plan := range plans {
		problems = append(problems, checkLaw(plan, in)...)
	}
	return problems
}

func duplicateSections(sections []LawPlan) []string {
	var problems []string
	seen := map[string]bool{}
	for _, section := range sections {
		if seen[section.Name] {
			problems = append(problems, fmt.Sprintf("law %s has more than one section", section.Name))
		}
		seen[section.Name] = true
	}
	return problems
}

func missingSections(sections []LawPlan, required map[string][]string) []string {
	documented := map[string]bool{}
	for _, section := range sections {
		documented[section.Name] = true
	}
	missing := map[string][]string{}
	for machine, laws := range required {
		addUndocumented(missing, machine, laws, documented)
	}
	var problems []string
	for _, law := range sortedKeys(missing) {
		sort.Strings(missing[law])
		problems = append(problems, fmt.Sprintf("law %s (machine %s) has no \"## %s\" section",
			law, strings.Join(missing[law], ", "), law))
	}
	return problems
}

// addUndocumented records, per law, the machines whose law has no section.
func addUndocumented(missing map[string][]string, machine string, laws []string, documented map[string]bool) {
	for _, law := range laws {
		if !documented[law] {
			missing[law] = append(missing[law], machine)
		}
	}
}

func checkLaw(plan LawPlan, in Inputs) []string {
	problems := append([]string(nil), plan.problems...)
	if err := checkLawSource(plan, in); err != nil {
		problems = append(problems, fmt.Sprintf("law %s %s", plan.Name, err))
	}
	for _, enforcer := range plan.Enforcers {
		if err := checkEnforcer(plan.Name, enforcer, in); err != nil {
			problems = append(problems, fmt.Sprintf("law %s %s", plan.Name, err))
		}
	}
	return append(problems, checkStatus(plan)...)
}

func checkLawSource(plan LawPlan, in Inputs) error {
	return checkSource(plan.Fields[FieldSource], in)
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
