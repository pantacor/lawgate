package report

import (
	"sort"
	"strings"
)

// The page is read top-down by someone deciding whether a change may go in:
// what needs attention comes first, as one list; then the machines, worst
// first, with the laws that hold folded away; then the gate checks, grouped
// by package with the shared module prefix cut, so thousands of checks read
// as a few dozen lines.

// htmlAttention is one law or check that is broken or pending, listed at the
// top of the page with where it lives.
type htmlAttention struct {
	Where      string // the machine or check group
	Name       string
	State      string
	Tone       string
	Detail     string
	Violations []string
}

// htmlGroup is a set of gate checks that share a package.
type htmlGroup struct {
	Name    string
	Checks  []htmlLaw
	Holds   int
	Broken  int
	Pending int
	Open    bool // a group with something broken or pending starts open
}

// count tallies the group's checks by state.
func (g *htmlGroup) count() {
	for _, check := range g.Checks {
		switch check.State {
		case "Broken":
			g.Broken++
		case "Pending":
			g.Pending++
		default:
			g.Holds++
		}
	}
	g.Open = g.Broken > 0
}

// splitLaws parts a card's laws into those that need attention and those
// that hold.
func splitLaws(laws []htmlLaw) (attention, holding []htmlLaw) {
	for _, law := range laws {
		if law.State == "Holds" {
			holding = append(holding, law)
			continue
		}
		attention = append(attention, law)
	}
	return attention, holding
}

// attentionOf lists what needs attention on one card.
func attentionOf(card htmlMachine) []htmlAttention {
	var out []htmlAttention
	if card.Failure != "" {
		out = append(out, htmlAttention{Where: card.Name, Name: "could not be checked", State: "Not checked", Tone: "danger", Detail: card.Failure})
	}
	for _, law := range card.Attention {
		where, name := card.Name, law.Name
		if card.GateChecks {
			where, name = checkGroup(law.Name)
			name = lawOfCheck(name)
		}
		out = append(out, htmlAttention{Where: where, Name: name, State: law.State, Tone: law.Tone, Detail: attentionDetail(law), Violations: law.Violations})
	}
	return out
}

// htmlReason gathers the attention items that share a reason, so forty
// laws waiting on the same kind of decision read as one line.
type htmlReason struct {
	Title string
	Tone  string
	Open  bool // broken items and short lists start open
	Items []htmlAttention
}

// groupAttention gathers items by reason, worst first.
func groupAttention(items []htmlAttention) []htmlReason {
	byTitle := map[string]*htmlReason{}
	var order []string
	for _, item := range items {
		title := reasonTitle(item)
		if byTitle[title] == nil {
			byTitle[title] = &htmlReason{Title: title, Tone: item.Tone}
			order = append(order, title)
		}
		byTitle[title].Items = append(byTitle[title].Items, item)
	}
	reasons := make([]htmlReason, 0, len(order))
	for _, title := range order {
		reason := *byTitle[title]
		reason.Open = reason.Tone == "danger" || len(reason.Items) <= 3
		reasons = append(reasons, reason)
	}
	sort.SliceStable(reasons, func(i, j int) bool {
		return severity(reasons[i].Tone) < severity(reasons[j].Tone)
	})
	return reasons
}

// reasonTitle is the reason an item needs attention, in a few words: the
// head of a pending explanation ("subject gap", "test did not run"), or its
// state.
func reasonTitle(item htmlAttention) string {
	if item.State != "Pending" {
		return item.State
	}
	head, _, _ := strings.Cut(item.Detail, ":")
	if head == "" || len(head) > 40 {
		return "Pending"
	}
	return "Pending: " + head
}

func attentionDetail(law htmlLaw) string {
	if law.Pending != "" {
		return law.Pending
	}
	return law.Doc
}

// sortAttention puts broken items before pending ones, keeping their order
// otherwise.
func sortAttention(items []htmlAttention) {
	sort.SliceStable(items, func(i, j int) bool {
		return severity(items[i].Tone) < severity(items[j].Tone)
	})
}

// sortCards puts cards that cannot be checked or have broken laws first,
// then pending ones, then the ones that hold.
func sortCards(cards []htmlMachine) {
	sort.SliceStable(cards, func(i, j int) bool {
		return severity(cards[i].Tone) < severity(cards[j].Tone)
	})
}

func severity(tone string) int {
	switch tone {
	case "danger":
		return 0
	case "info", "warning":
		return 1
	}
	return 2
}

// groupChecks cuts the prefix every check name shares and groups the
// checks by package. It returns the prefix it cut.
func groupChecks(checks []htmlLaw) (string, []htmlGroup) {
	prefix := sharedPrefix(checks)
	byName := map[string]*htmlGroup{}
	var order []string
	for _, check := range checks {
		check.Name = cutPrefix(check.Name, prefix)
		group, short := checkGroup(check.Name)
		check.Name = short
		if byName[group] == nil {
			byName[group] = &htmlGroup{Name: group}
			order = append(order, group)
		}
		byName[group].Checks = append(byName[group].Checks, check)
	}
	sort.Strings(order)
	groups := make([]htmlGroup, 0, len(order))
	for _, name := range order {
		group := *byName[name]
		group.count()
		groups = append(groups, group)
	}
	return prefix, groups
}

// trimPrefix cuts prefix from the names of checks that need attention, so
// they read like the grouped ones.
func trimPrefix(prefix string, laws []htmlLaw) []htmlLaw {
	out := make([]htmlLaw, len(laws))
	for i, law := range laws {
		law.Name = cutPrefix(law.Name, prefix)
		out[i] = law
	}
	return out
}

// cutPrefix drops prefix wherever it appears in name, so a check that names
// a function by its full path ("subject example.com/app/pkg.F") reads short.
func cutPrefix(name, prefix string) string {
	if prefix == "" {
		return name
	}
	return strings.ReplaceAll(name, prefix, "")
}

// sharedPrefix is the longest prefix, ending at a "/", that the names of
// the Go test checks share; checks with other names don't count.
func sharedPrefix(checks []htmlLaw) string {
	prefix, seen := "", false
	for _, check := range checks {
		if !isTestPath(check.Name) {
			continue
		}
		if !seen {
			prefix, seen = check.Name, true
			continue
		}
		prefix = commonPrefix(prefix, check.Name)
	}
	if cut := strings.LastIndex(prefix, "/"); cut >= 0 {
		return prefix[:cut+1]
	}
	return ""
}

// isTestPath reports whether name is a Go test's full name: an import path
// whose first element is a host ("example.com/app/pkg/TestX"). File names
// such as "modules/x/LAWS.md" don't count.
func isTestPath(name string) bool {
	if strings.Contains(name, " ") {
		return false
	}
	host, _, found := strings.Cut(name, "/")
	return found && strings.Contains(host, ".")
}

func commonPrefix(a, b string) string {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return a[:i]
		}
	}
	return a[:n]
}

// checkGroup names the group a check belongs to and the name it shows in
// that group: a Go test groups by its package ("modules/api/devices" for
// "modules/api/devices/TestX/case"), a named repository check by the word
// before its parenthesis ("laws-bite" for "laws-bite (law: …)").
func checkGroup(name string) (group, short string) {
	if open := strings.Index(name, " ("); open > 0 {
		return name[:open], name[open+1:]
	}
	parts := strings.Split(name, "/")
	for i, part := range parts {
		if isTestName(part) && i > 0 {
			return strings.Join(parts[:i], "/"), strings.Join(parts[i:], "/")
		}
	}
	return "repository", name
}

func isTestName(part string) bool {
	for _, kind := range []string{"Test", "Fuzz", "Example", "Benchmark"} {
		if strings.HasPrefix(part, kind) {
			return true
		}
	}
	return false
}

// lawOfCheck shortens a check about one law ("(some-law: LAWS.md, no
// subject)") to the law's name; other names stay as they are.
func lawOfCheck(name string) string {
	if !strings.HasPrefix(name, "(") || !strings.HasSuffix(name, ")") {
		return name
	}
	law, _, _ := strings.Cut(strings.Trim(name, "()"), ":")
	return law
}
