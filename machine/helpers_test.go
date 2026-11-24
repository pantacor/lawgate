package machine

// Small constructors keep test machines flat: each part is built by name and
// assembled, instead of one nested literal.

func next(to string) Edge { return Edge{On: "to " + to, To: to, Kind: EdgeNext} }

func named(edge Edge, name string) Edge {
	edge.Name = name
	return edge
}

func failSite(tag string) Site { return Site{Tag: tag, Kind: SiteFail, Effect: "told"} }

func step(id string, sites ...Site) Step { return Step{ID: id, Kind: "computer", Sites: sites} }

func working(id string, edges ...Edge) State { return State{ID: id, Edges: edges} }

func withSteps(st State, steps ...Step) State {
	st.Steps = steps
	return st
}

func terminal(id string) State { return State{ID: id, Terminal: true} }

func newMachine(states ...State) *Machine { return &Machine{Name: "m", States: states} }

func catchTo(kind SiteKind, to string) Catch { return Catch{On: kind, To: to} }
