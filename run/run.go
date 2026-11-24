// Package run records the accepted moves of one pass through a declared
// machine. Generated state types use it to record their transitions. An
// invalid move is reported as drift and leaves the state and trace intact.
package run

import (
	"errors"
	"log"

	"github.com/pantacor/lawgate/machine"
)

// EnterEdge is the Edge of the first move of a pass.
const EnterEdge = machine.EntryEdgeName

// Transition is one move of a pass. An ordinary move names its Edge; a
// catch move sets Catch instead. The first move has Edge set to EnterEdge.
type Transition struct {
	From, Edge, To string
	Catch          machine.SiteKind
}

// Run is one pass of Machine.
type Run struct {
	Machine *machine.Machine
	Trace   []Transition
	Drift   []string
	// OnDrift reports a rejected move. Without it, drift is logged.
	OnDrift func(msg string)
	cursor  cursor
}

// New starts recording a pass of m.
func New(m *machine.Machine) *Run { return &Run{Machine: m} }

// At is the state the pass is in; "" before Enter.
func (r *Run) At() string { return r.cursor.at }

// Enter starts the pass in the entry state or a declared pass entry.
func (r *Run) Enter(id string) error {
	return r.record(Transition{Edge: EnterEdge, To: id})
}

// Go records a declared edge if the run is in its source state and its
// count in this pass is below any declared limit. The edge needs a name:
// an empty one would match an edge declared without one, and the trace
// would hold a move that names nothing.
func (r *Run) Go(from, edge, to string) error {
	if edge == EnterEdge {
		return r.reject(errors.New("enter is reserved for the start of a pass"))
	}
	return r.record(Transition{From: from, Edge: edge, To: to})
}

// Catch records a declared catch from a state with a site of that kind, as
// long as its count in this pass is below the catch's declared limit.
func (r *Run) Catch(from string, kind machine.SiteKind, to string) error {
	if kind == "" {
		return r.reject(errors.New("catch move has no site kind"))
	}
	return r.record(Transition{From: from, Catch: kind, To: to})
}

func (r *Run) record(move Transition) error {
	if r == nil {
		return errors.New("cannot record a move without a run")
	}
	if err := r.cursor.advance(r.Machine, move); err != nil {
		return r.reject(err)
	}
	r.Trace = append(r.Trace, move)
	return nil
}

func (r *Run) reject(err error) error {
	if r != nil {
		r.drift(err.Error())
	}
	return err
}

func (r *Run) drift(msg string) {
	r.Drift = append(r.Drift, msg)
	if r.OnDrift != nil {
		r.OnDrift(msg)
		return
	}
	log.Printf("MACHINE_DRIFT -- %s", msg)
}
