package render

import (
	"fmt"
	"strings"

	"github.com/pantacor/lawgate/machine"
)

// summaryKinds is the order Summary counts site kinds in.
var summaryKinds = []machine.SiteKind{
	machine.SiteFail, machine.SiteFault, machine.SiteSilent, machine.SiteWarn,
	machine.SiteWrite, machine.SiteLog, machine.SiteGuard,
}

// Summary is a one-line count of states and of sites by kind, for example
// "4 states, 1 fail, 2 fault, 0 silent, 1 write, 0 log, 0 guard". A kind
// with no site is left out only for warn, which most machines lack.
func Summary(m *machine.Machine) string {
	counts := map[machine.SiteKind]int{}
	m.Sites(func(_ *machine.State, _ *machine.Step, site *machine.Site) { counts[site.Kind]++ })
	parts := []string{fmt.Sprintf("%d states", len(m.States))}
	for _, kind := range summaryKinds {
		noWarnings := kind == machine.SiteWarn && counts[kind] == 0
		if !noWarnings {
			parts = append(parts, fmt.Sprintf("%d %s", counts[kind], kind))
		}
	}
	return strings.Join(parts, ", ")
}
