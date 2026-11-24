package laws

import (
	"slices"
	"strings"

	"github.com/pantacor/lawgate/machine"
)

// NothingSilentName is the name of the law NothingSilent returns.
const NothingSilentName = "nothing-silent"

// EveryFailureSaysWhyName is the name of the law EveryFailureSaysWhy returns.
const EveryFailureSaysWhyName = "every-failure-says-why"

// NothingSilent is the law "nothing-silent": every failure reaches the user,
// so no site is machine.SiteSilent (one that only writes to a log and carries
// on). A catch on silent sites that carries on (machine.Catch, for example
// Fleet's "pod log only") only says where the log goes: the sites still
// break the law. Its witness adds a silent site to the entry state.
func NothingSilent() machine.Law {
	return machine.Law{
		Name:    NothingSilentName,
		Doc:     "every failure reaches the user: no site only writes to the log",
		Check:   checkNothingSilent,
		Witness: addSilentSite,
	}
}

func checkNothingSilent(m *machine.Machine) []string {
	var violations []string
	m.Sites(func(st *machine.State, step *machine.Step, site *machine.Site) {
		if site.Kind == machine.SiteSilent {
			violations = append(violations, machine.Siteline(st, step, site)+" is silent")
		}
	})
	return violations
}

func addSilentSite(m *machine.Machine) {
	addEntryStep(m, machine.Site{Tag: witnessID, Kind: machine.SiteSilent, Effect: "logged only"})
}

// DefaultFailureKinds are the site kinds EveryFailureSaysWhy checks when it
// is given none: the sites that end, retry or partly fail a job.
var DefaultFailureKinds = []machine.SiteKind{machine.SiteFail, machine.SiteFault, machine.SiteWarn}

// EveryFailureSaysWhy is the law "every-failure-says-why": every site of
// one of kinds declares, in its Effect, what the user is told. With no kinds
// it checks DefaultFailureKinds; a product whose failed writes reach the user
// adds machine.SiteWrite. Its witness adds a site of the first kind with no
// effect to the entry state.
func EveryFailureSaysWhy(kinds ...machine.SiteKind) machine.Law {
	checked := slices.Clone(kinds)
	if len(checked) == 0 {
		checked = slices.Clone(DefaultFailureKinds)
	}
	return machine.Law{
		Name:    EveryFailureSaysWhyName,
		Doc:     "every " + joinKinds(checked) + " site declares the effect the user is told about",
		Check:   func(m *machine.Machine) []string { return unexplainedSites(m, checked) },
		Witness: func(m *machine.Machine) { addEntryStep(m, machine.Site{Tag: witnessID, Kind: checked[0]}) },
	}
}

func unexplainedSites(m *machine.Machine, kinds []machine.SiteKind) []string {
	var violations []string
	m.Sites(func(st *machine.State, step *machine.Step, site *machine.Site) {
		unexplained := slices.Contains(kinds, site.Kind) && strings.TrimSpace(site.Effect) == ""
		if unexplained {
			violations = append(violations, machine.Siteline(st, step, site)+" does not say what the user is told")
		}
	})
	return violations
}

func joinKinds(kinds []machine.SiteKind) string {
	names := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		names = append(names, string(kind))
	}
	return strings.Join(names, ", ")
}
