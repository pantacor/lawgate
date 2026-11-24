package shop

import (
	"regexp"

	"github.com/pantacor/lawgate/machine"
	"github.com/pantacor/lawgate/resolve"
	"github.com/pantacor/lawgate/runner"
)

// Entry is the pay machine as cmd/machine checks it.
func Entry() runner.Entry {
	calls := map[string]machine.SiteKind{"fail": machine.SiteFail, "record": machine.SiteWrite}
	vocabulary := resolve.Vocabulary{TagPattern: regexp.MustCompile(`^(SHOP_JOB \d+)`), Calls: calls}
	return runner.Entry{Machine: Machine(), Vocabulary: vocabulary, Prefix: "SHOP_JOB"}
}

// Machine declares Pay.
func Machine() *machine.Machine {
	sites := []machine.Site{
		{Tag: "1", Kind: machine.SiteFail, Effect: "refuses the payment (law never-overspend)"},
		{Tag: "2", Kind: machine.SiteWrite},
	}
	steps := []machine.Step{{ID: "charge", Kind: "store", Sites: sites}}
	edges := []machine.Edge{{On: "ok", To: "done", Kind: machine.EdgeExit}}
	catches := []machine.Catch{
		{On: machine.SiteFail, To: "failed"},
		{On: machine.SiteWrite, To: "failed"},
	}
	states := []machine.State{
		{ID: "pay", Func: "Pay", Doc: "Charges the order.", Steps: steps, Edges: edges},
		{ID: "done", Terminal: true},
		{ID: "failed", Terminal: true},
	}
	law := machine.Law{Name: "no-silent-errors", Check: noSilentSites, Witness: silenceFirstSite}
	return &machine.Machine{Name: "pay", Source: "worker.go", Laws: []machine.Law{law}, Catches: catches, States: states}
}

func noSilentSites(m *machine.Machine) []string {
	var violations []string
	m.Sites(func(state *machine.State, step *machine.Step, site *machine.Site) {
		if site.Kind == machine.SiteSilent {
			violations = append(violations, machine.Siteline(state, step, site))
		}
	})
	return violations
}

func silenceFirstSite(m *machine.Machine) {
	m.States[0].Steps[0].Sites[0].Kind = machine.SiteSilent
}
