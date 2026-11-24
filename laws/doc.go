// Package laws offers the machine laws worker products share, as
// constructors that return a machine.Law with its Check and its Witness.
//
// The laws keep the names adopting repositories already use in their
// LAWS.md files ("nothing-silent", "done-means-stored", ...), so a product
// moves onto them without renaming a section. Product facts the laws need
// (the done and failed state ids, a site label such as "stores-done", a
// machine label such as "one-shot") are parameters: the package carries no
// product vocabulary.
//
// Each Witness changes an isolated copy of a typical worker machine so the
// law breaks on it: it adds a state, a step or a site with a fresh id, and
// never renames the machine or moves its entry state. machine.CheckLaws runs
// the witness of every law that holds and refuses one that does not break.
//
// A law's Doc is a default; a product that words it differently sets Doc on
// the returned value. The name is the law's identity and stays.
package laws
