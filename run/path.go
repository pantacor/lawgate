package run

import (
	"errors"
	"fmt"

	"github.com/pantacor/lawgate/machine"
)

// ErrEmptyTrace is returned by IsPath when there is no move to check.
var ErrEmptyTrace = errors.New("the trace is empty")

// ErrLimit is wrapped when a move exceeds its declared limit in one pass.
var ErrLimit = errors.New("limit reached")

// IsPath checks that trace is one pass of m. Replay uses the same entry,
// edge, catch, and limit checks as live recording.
func IsPath(m *machine.Machine, trace []Transition) error {
	if len(trace) == 0 {
		return ErrEmptyTrace
	}
	var path cursor
	for i, move := range trace {
		if err := path.advance(m, move); err != nil {
			return fmt.Errorf("move %d: %w", i, err)
		}
	}
	return nil
}

// Both live runs and replay use a fresh cursor for each pass.
type cursor struct {
	started bool
	at      string
	counts  map[Transition]int
}

func (c *cursor) advance(m *machine.Machine, move Transition) error {
	if m == nil {
		return errors.New("cannot validate a move without a machine")
	}
	if !c.started {
		return c.enter(m, move)
	}
	limit, err := c.moveLimit(m, move)
	if err != nil {
		return err
	}
	if err := c.countLimitedMove(move, limit); err != nil {
		return err
	}
	c.at = move.To
	return nil
}

// countLimitedMove counts move against limit, 0 when the declaration sets
// none. An edge and a catch are each counted per source state.
func (c *cursor) countLimitedMove(move Transition, limit int) error {
	if limit <= 0 {
		return nil
	}
	count := c.counts[move]
	if count >= limit {
		return fmt.Errorf("%w: %s taken %d times, limit %d", ErrLimit, describe(move), count, limit)
	}
	if c.counts == nil {
		c.counts = make(map[Transition]int)
	}
	c.counts[move] = count + 1
	return nil
}

func (c *cursor) enter(m *machine.Machine, move Transition) error {
	entryMove := move.Edge == EnterEdge && move.From == "" && move.Catch == ""
	if !entryMove {
		return fmt.Errorf("pass of %s must begin with an enter move, got %+v", m.Name, move)
	}
	st := m.State(move.To)
	if st == nil {
		return fmt.Errorf("entered state %q, which the machine %s does not declare", move.To, m.Name)
	}
	if move.To != m.EntryID() && !st.PassEntry {
		return fmt.Errorf("entered state %q, which is neither the entry state of %s nor a pass entry", move.To, m.Name)
	}
	c.started, c.at = true, move.To
	return nil
}

// moveLimit returns how many times move may be taken in this pass, 0 when
// the declaration sets no limit, once the pass may take it from where it is.
func (c *cursor) moveLimit(m *machine.Machine, move Transition) (int, error) {
	if err := c.checkContinuity(m, move); err != nil {
		return 0, err
	}
	from, to := m.State(move.From), m.State(move.To)
	if from == nil || to == nil {
		return 0, fmt.Errorf("move between undeclared states in %s: %+v", m.Name, move)
	}
	if move.Catch != "" {
		return catchLimit(m, from, move)
	}
	return edgeLimit(from, move, m.Name)
}

// checkContinuity refuses a move the pass cannot make where it is: a second
// entry, a move that leaves a state the pass is not in, or one that is
// neither a named edge nor a catch.
func (c *cursor) checkContinuity(m *machine.Machine, move Transition) error {
	if move.Edge == EnterEdge {
		return fmt.Errorf("entered %q in the middle of the pass of %s, which is in %q", move.To, m.Name, c.at)
	}
	if move.From != c.at {
		return fmt.Errorf("move leaves %q but the pass of %s is in %q", move.From, m.Name, c.at)
	}
	return checkMoveShape(move)
}

// checkMoveShape refuses a move that is not exactly one of the two kinds a
// trace records: a named edge, or a catch on a site kind. An empty edge name
// used to match an edge declared without one, which left a trace
// move that named nothing.
func checkMoveShape(move Transition) error {
	isCatch := move.Catch != ""
	hasEdgeName := move.Edge != ""
	switch {
	case isCatch && hasEdgeName:
		return fmt.Errorf("move from %q to %q names both edge %q and catch %s", move.From, move.To, move.Edge, move.Catch)
	case !isCatch && !hasEdgeName:
		return fmt.Errorf("move from %q to %q names neither an edge nor a catch", move.From, move.To)
	}
	return nil
}

// catchLimit returns the limit of the catch rule move follows, which the
// source state must hold a site of. Without a limit, a state catching its
// own fault back into itself would loop.
func catchLimit(m *machine.Machine, from *machine.State, move Transition) (int, error) {
	catch, declared := catchRule(m, move.Catch, move.To)
	if !declared || !from.Holds(move.Catch) {
		return 0, fmt.Errorf("%s is not declared by %s", describe(move), m.Name)
	}
	return catch.Limit, nil
}

// carryOn checks a catch that carries on from where the pass is. It moves
// nothing and counts nothing: a carry-on catch has no limit (Validate).
func (c *cursor) carryOn(m *machine.Machine, from string, kind machine.SiteKind) error {
	if m == nil {
		return errors.New("cannot validate a catch without a machine")
	}
	if !c.started || from != c.at {
		return fmt.Errorf("catch on %s carries on in %q but the pass of %s is in %q", kind, from, m.Name, c.at)
	}
	_, declared := catchRule(m, kind, "")
	if !declared || !m.State(from).Holds(kind) {
		return fmt.Errorf("%s --catch %s--> (carries on) is not declared by %s", from, kind, m.Name)
	}
	return nil
}

// catchRule is the machine's catch rule for a site of kind leading to to.
func catchRule(m *machine.Machine, kind machine.SiteKind, to string) (machine.Catch, bool) {
	for _, catch := range m.Catches {
		if catch.On == kind && catch.To == to {
			return catch, true
		}
	}
	return machine.Catch{}, false
}

// edgeLimit returns the limit of the edge of from that move names.
func edgeLimit(from *machine.State, move Transition, machineName string) (int, error) {
	for _, edge := range from.Edges {
		if edge.Name == move.Edge && edge.To == move.To {
			return edge.Limit, nil
		}
	}
	return 0, fmt.Errorf("%s is not declared by %s", describe(move), machineName)
}

// describe names a move for errors: --edge--> or --catch kind-->.
func describe(move Transition) string {
	if move.Catch != "" {
		return fmt.Sprintf("%s --catch %s--> %s", move.From, move.Catch, move.To)
	}
	return fmt.Sprintf("%s --%s--> %s", move.From, move.Edge, move.To)
}
