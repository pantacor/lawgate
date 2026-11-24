// Package machine declares a worker's control flow as data: the states a job
// moves through, the steps each state runs, the edges between them and what
// happens on each failure. A declaration runs nothing. It is a spec that sits
// beside the imperative code and is checked against it (package resolve) and
// against laws (Law, CheckLaws).
//
// The package carries no product vocabulary. A product names its own step
// kinds, statuses and labels; the site kinds below are the framework's own,
// because the shared laws reason about them.
package machine

// StepKind says who performs a step. Each product names its own kinds (for
// example "db", "hub", "runtime"); the framework only compares them.
type StepKind string

// SiteKind says what a failure site does with the error.
type SiteKind string

const (
	// SiteFault returns an error the job runner handles: the attempt ends and
	// is retried under the product's fault policy.
	SiteFault SiteKind = "fault"
	// SiteFail ends the job as failed, with a message the user sees.
	SiteFail SiteKind = "fail"
	// SiteWrite stores a status. A failed write becomes a fault.
	SiteWrite SiteKind = "write"
	// SiteSilent logs the error and carries on: nobody but the log sees it.
	SiteSilent SiteKind = "silent"
	// SiteWarn records the error where the user sees it and carries on: a
	// partial result, said out loud.
	SiteWarn SiteKind = "warn"
	// SiteLog only logs; it is not an error path.
	SiteLog SiteKind = "log"
	// SiteGuard is an assertion the callers already make impossible. It is
	// drawn dimmed, not as a failure path, and may not sit on a call that
	// reports a failure (package resolve refuses that).
	SiteGuard SiteKind = "guard"
)

// EdgeKind says how control leaves a state.
type EdgeKind string

const (
	EdgeNext EdgeKind = "next" // continue in this pass
	EdgeLoop EdgeKind = "loop" // come back to an earlier state
	EdgeExit EdgeKind = "exit" // terminal or parked: the pass ends
)

// Machine is one worker's declared control flow.
type Machine struct {
	Name   string `json:"name"`
	Source string `json:"source"` // file the sites are resolved in, relative to the module root
	Doc    string `json:"doc"`
	// Entry is the state every job starts in. Empty means the first state.
	Entry string `json:"entry,omitempty"`
	// Labels are product facts about the whole machine (for example
	// "one-shot"); product laws read them, the framework does not.
	Labels []string `json:"labels,omitempty"`
	// Laws are the machine's own laws, checked next to the product's shared
	// ones.
	Laws []Law `json:"-"`
	// Test is the command that checks the declaration against the source.
	Test    string  `json:"test,omitempty"`
	Catches []Catch `json:"catches"`
	States  []State `json:"states"`
	// Findings are problems the declaration surfaced while it was written.
	Findings []Finding `json:"findings,omitempty"`
}

// Catch is a machine-wide rule for a class of failure: every site of kind On
// leads to state To.
//
// A catch with an empty To carries on: the pass stays where it is and goes
// on with the next step, and Note says where the error is logged (for
// example "the action's log", "the pod log only"). It moves the job nowhere,
// so the graph helpers see no edge for it and package run records no move.
// Validate refuses a non-empty To that names no state, and a Limit on a
// carry-on catch, which bounds nothing.
type Catch struct {
	On   SiteKind `json:"on"`
	To   string   `json:"to"`
	Note string   `json:"note"`
	// Limit, when above zero, is how many times this catch may be taken out
	// of one state in one pass, counted like Edge.Limit: package run
	// enforces it during recording and replay. A catch whose target holds
	// the caught kind loops, so Validate requires a limit on it.
	Limit int `json:"limit,omitempty"`
}

// CarriesOn reports whether the catch leaves the pass where it is: its To is
// empty.
func (c Catch) CarriesOn() bool { return c.To == "" }

// State is a phase of a pass. A terminal state ends the job; a wait state
// ends the pass and parks the job until it is due again.
type State struct {
	ID     string `json:"id"`
	Func   string `json:"func,omitempty"`   // Go function that implements it
	Status string `json:"status,omitempty"` // status the product shows while here
	When   string `json:"when,omitempty"`   // entry condition, in words
	Doc    string `json:"doc,omitempty"`
	// Terminal states end the job: done, failed, canceled.
	Terminal bool `json:"terminal,omitempty"`
	// Wait states end the pass and park the job.
	Wait bool `json:"wait,omitempty"`
	// PassEntry marks a state a later pass may start in (the product picks
	// it from the stored status). Package run refuses a pass that starts
	// anywhere else than the entry state or a pass entry.
	PassEntry bool `json:"pass_entry,omitempty"`
	// WokenBy names what wakes a wait that has no limit of its own.
	WokenBy string `json:"woken_by,omitempty"`
	Steps   []Step `json:"steps,omitempty"`
	Edges   []Edge `json:"edges,omitempty"`
}

// Step is one unit of work inside a state. Steps run in declaration order;
// a site's catch may leave the state from its step.
type Step struct {
	ID    string   `json:"id"`
	Kind  StepKind `json:"kind"`
	Doc   string   `json:"doc,omitempty"`
	Sites []Site   `json:"sites,omitempty"`
}

// Site is a place in the source where the step can fail or log. Tag is the
// log tag without the machine prefix ("23", "(integrity)", "-" for the bare
// prefix). Match picks the line by substring of code (comments never count),
// alone for untagged sites or together with Tag when one tag appears twice.
type Site struct {
	Tag    string   `json:"tag,omitempty"`
	Match  string   `json:"match,omitempty"`
	Kind   SiteKind `json:"kind"`
	Effect string   `json:"effect,omitempty"`
	// Labels are product facts about the site (for example "stores-done").
	// Package resolve checks each label against the calls the product ties
	// to it.
	Labels []string `json:"labels,omitempty"`
	Line   int      `json:"line,omitempty"` // filled by package resolve
}

// Edge is a way out of a state. Edges leave after the state's last step.
type Edge struct {
	On    string   `json:"on"` // the outcome that takes it
	To    string   `json:"to"` // target state id
	Kind  EdgeKind `json:"kind"`
	Guard string   `json:"guard,omitempty"` // limit or condition, in words
	// Limit, when above zero, is how many times this edge may be taken in
	// one pass. Package run enforces it during recording and replay.
	Limit int `json:"limit,omitempty"`
	// Name is the Go method the generated code gives this edge (package
	// gen). Every edge of a generated state needs one.
	Name string `json:"name,omitempty"`
}

// Finding is a defect the declaration made visible.
type Finding struct {
	Severity string `json:"severity"` // high, medium, low
	Tag      string `json:"tag,omitempty"`
	Match    string `json:"match,omitempty"`
	Line     int    `json:"line,omitempty"`
	Text     string `json:"text"`
}

// EntryID is the state every job starts in.
func (m *Machine) EntryID() string {
	if m.Entry != "" {
		return m.Entry
	}
	if len(m.States) == 0 {
		return ""
	}
	return m.States[0].ID
}

// State looks a state up by id; nil when it is not declared.
func (m *Machine) State(id string) *State {
	for i := range m.States {
		if m.States[i].ID == id {
			return &m.States[i]
		}
	}
	return nil
}

// HasLabel reports whether the machine carries label.
func (m *Machine) HasLabel(label string) bool {
	return containsString(m.Labels, label)
}

// HasLabel reports whether the site carries label.
func (s *Site) HasLabel(label string) bool {
	return containsString(s.Labels, label)
}

// Sites walks every site in declaration order.
func (m *Machine) Sites(visit func(state *State, step *Step, site *Site)) {
	for i := range m.States {
		m.States[i].sites(visit)
	}
}

func (st *State) sites(visit func(state *State, step *Step, site *Site)) {
	for j := range st.Steps {
		step := &st.Steps[j]
		for k := range step.Sites {
			visit(st, step, &step.Sites[k])
		}
	}
}

// Holds reports whether the state has a site of kind.
func (st *State) Holds(kind SiteKind) bool {
	found := false
	st.sites(func(_ *State, _ *Step, site *Site) {
		found = found || site.Kind == kind
	})
	return found
}

// EdgeTo reports whether the state has an edge straight to state to.
func (st *State) EdgeTo(to string) bool {
	for _, edge := range st.Edges {
		if edge.To == to {
			return true
		}
	}
	return false
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
