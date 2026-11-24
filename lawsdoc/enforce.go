package lawsdoc

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/pantacor/lawgate/bite"
)

// Kind identifies an enforcer or a documented gap.
type Kind int

const (
	Machine Kind = iota + 1
	Test
	Script
	NotYet
	Proposed
)

type Enforcer struct {
	Kind Kind
	Name string // machine, test, script path, or the reason for a gap
}

type LawPlan struct {
	Section
	Enforcers []Enforcer
	BiteGap   string // reason an agreed test or script has no breaking case
	problems  []string
}

var namedEnforcer = regexp.MustCompile("\\b(machine|test|script) `([^`]+)`")

// Plan reads a document once for source checks and for the runners.
func Plan(doc string) []LawPlan {
	var plans []LawPlan
	for _, section := range Parse(doc) {
		plans = append(plans, planLaw(section))
	}
	return plans
}

func planLaw(section Section) LawPlan {
	plan := LawPlan{Section: section}
	if section.Reserved() {
		// No enforcer is read: the gate must never rerun a test with a
		// selector that a framework hook answers to.
		plan.fail("uses the reserved %q prefix: lawgate keeps it for its own bite selectors, rename the law", bite.FrameworkPrefix)
		return plan
	}
	value := section.Fields[FieldEnforcedBy]
	switch {
	case value == "":
		plan.fail("has no \"- Enforced by:\" line")
	case strings.HasPrefix(value, "not yet"):
		plan.readEnforcementGap(value)
	case value == "proposed", strings.HasPrefix(value, "proposed "):
		enforcer := Enforcer{Kind: Proposed}
		plan.Enforcers = []Enforcer{enforcer}
	default:
		plan.Enforcers = namedEnforcers(value)
		if len(plan.Enforcers) == 0 {
			plan.fail("\"Enforced by: %s\" names no machine, test or script, and is neither \"not yet\" nor \"proposed\"", value)
		}
	}
	plan.readBiteGap()
	return plan
}

func (plan *LawPlan) readEnforcementGap(value string) {
	reason := plan.gapReason(FieldEnforcedBy, value)
	enforcer := Enforcer{Kind: NotYet, Name: reason}
	plan.Enforcers = []Enforcer{enforcer}
}

// EnforcementGap returns the reason the law has no enforcer, if recorded.
func (plan LawPlan) EnforcementGap() string {
	if len(plan.Enforcers) == 1 && plan.Enforcers[0].Kind == NotYet {
		return plan.Enforcers[0].Name
	}
	return ""
}

func (plan *LawPlan) readBiteGap() {
	value, present := plan.Fields[FieldBites]
	if !present {
		return
	}
	plan.BiteGap = plan.gapReason(FieldBites, value)
	if plan.BiteGap != "" && !slices.ContainsFunc(plan.Enforcers, Enforcer.CanBite) {
		plan.fail("\"Bites:\" records a gap but the law has no test or script enforcer")
	}
}

func (plan *LawPlan) gapReason(field, value string) string {
	if value == "not yet." {
		plan.fail("with \"%s: not yet.\" must say why", field)
		return ""
	}
	reason, valid := strings.CutPrefix(value, "not yet. ")
	if !valid || strings.Contains(value, "\n") {
		plan.fail("\"%s:\" must be \"not yet. <why>\"", field)
		return ""
	}
	return strings.TrimSpace(reason)
}

// CanBite reports whether this enforcer participates in the bite rerun phase.
func (enforcer Enforcer) CanBite() bool {
	return enforcer.Kind == Test || enforcer.Kind == Script
}

// Agreed reports whether the law has an agreed status.
func (plan LawPlan) Agreed() bool {
	date := statusAgreedRE.FindStringSubmatch(plan.Fields[FieldStatus])
	return len(date) != 0 && validDate(date[1])
}

func (plan *LawPlan) fail(format string, args ...any) {
	plan.problems = append(plan.problems, fmt.Sprintf("law %s ", plan.Name)+fmt.Sprintf(format, args...))
}

func namedEnforcers(value string) []Enforcer {
	var enforcers []Enforcer
	for _, match := range namedEnforcer.FindAllStringSubmatch(value, -1) {
		kind := Script
		switch match[1] {
		case "machine":
			kind = Machine
		case "test":
			kind = Test
		}
		enforcers = append(enforcers, Enforcer{Kind: kind, Name: match[2]})
	}
	return enforcers
}

func checkEnforcer(law string, enforcer Enforcer, in Inputs) error {
	name := enforcer.Name
	switch enforcer.Kind {
	case Machine:
		return checkMachineEnforcer(law, name, in)
	case Test:
		if in.CheckTest != nil {
			return in.CheckTest(law, name)
		}
		_, err := in.Tests.EnforcingTest(law, name)
		return err
	case Script:
		_, err := ScriptPath(in.RepoRoot, in.DocPath, name)
		return err
	}
	return nil
}

func checkMachineEnforcer(law, name string, in Inputs) error {
	if name == "*" && in.SharedMachines != nil {
		return checkAdoptingMachines(law, in)
	}
	return checkMachineEvidence(law, name, in.Machines)
}

func checkAdoptingMachines(law string, in Inputs) error {
	if len(in.SharedMachines) == 0 {
		return fmt.Errorf("is enforced by machine %q, but the module has no expected machines", "*")
	}
	for _, name := range in.SharedMachines {
		if err := checkMachineEvidence(law, name, in.Machines); err != nil {
			return err
		}
	}
	return nil
}

func checkMachineEvidence(law, name string, machines map[string][]string) error {
	laws, registered := machines[name]
	if !registered {
		return fmt.Errorf("is enforced by machine %q, which is not a registered machine or has no verdicts", name)
	}
	if !slices.Contains(laws, law) {
		return fmt.Errorf("is enforced by machine %q, which has no law of that name in its verdicts", name)
	}
	return nil
}

func checkStatus(plan LawPlan) []string {
	status := plan.Fields[FieldStatus]
	switch {
	case status == "":
		return []string{fmt.Sprintf("law %s has no \"- Status:\" line", plan.Name)}
	case status == "proposed":
		return nil
	case !statusAgreedRE.MatchString(status):
		return []string{fmt.Sprintf("law %s: \"Status: %s\" is neither \"agreed <YYYY-MM-DD>\" nor \"proposed\"", plan.Name, status)}
	case !plan.Agreed():
		return []string{fmt.Sprintf("law %s: \"Status: %s\" must use a valid calendar date", plan.Name, status)}
	case len(plan.Enforcers) == 1 && plan.Enforcers[0].Kind == Proposed:
		return []string{fmt.Sprintf("law %s is agreed but nothing enforces it (Enforced by: proposed)", plan.Name)}
	}
	return nil
}

// MachineNames returns the machines this law requires evidence from.
func (plan LawPlan) MachineNames() []string {
	var names []string
	for _, enforcer := range plan.Enforcers {
		if enforcer.Kind == Machine {
			names = append(names, enforcer.Name)
		}
	}
	return names
}
