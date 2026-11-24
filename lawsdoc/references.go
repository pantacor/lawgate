package lawsdoc

import "slices"

type References struct {
	Tests   []string
	Scripts []string
}

func Enforcers(document string) References {
	var refs References
	for _, law := range Plan(document) {
		for _, enforcer := range law.Enforcers {
			refs.add(enforcer)
		}
	}
	slices.Sort(refs.Tests)
	slices.Sort(refs.Scripts)
	refs.Tests = slices.Compact(refs.Tests)
	refs.Scripts = slices.Compact(refs.Scripts)
	return refs
}

func (refs *References) add(enforcer Enforcer) {
	switch enforcer.Kind {
	case Test:
		refs.Tests = append(refs.Tests, enforcer.Name)
	case Script:
		refs.Scripts = append(refs.Scripts, enforcer.Name)
	}
}
