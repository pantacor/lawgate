package runner

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/pantacor/lawgate/render"
)

// runOptions are the runner's flags. At most one of laws, index, list, json
// and brief asks for an action; with none the runner prints the outline.
// machine narrows any action to one machine.
type runOptions struct {
	laws    bool
	junit   string
	index   string
	list    bool
	json    bool
	brief   string
	color   bool
	machine string
}

func parseArgs(args []string, stderr io.Writer) (runOptions, bool) {
	var options runOptions
	fs := flag.NewFlagSet("machine", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.BoolVar(&options.laws, "laws", false, "check declared machine laws")
	fs.StringVar(&options.junit, "junit", "", "with -laws: write JUnit XML report to file")
	fs.StringVar(&options.index, "index", "", "write the machine index for lawgate review to `file` (- for stdout)")
	fs.BoolVar(&options.list, "list", false, "list the machines with their state and law counts")
	fs.BoolVar(&options.json, "json", false, "write one machine, its drift, laws and source lines as JSON")
	fs.StringVar(&options.brief, "brief", "", "write the brief an agent needs to change `state`")
	fs.BoolVar(&options.color, "color", false, "colour the outline for a terminal")
	fs.StringVar(&options.machine, "machine", "", "work on the machine `name` only")
	if err := fs.Parse(args); err != nil {
		return options, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "machine runner accepts flags only")
		return options, false
	}
	if problem := options.problem(); problem != "" {
		fmt.Fprintln(stderr, problem)
		return options, false
	}
	return options, true
}

// problem says why the flags do not ask for one thing.
func (options runOptions) problem() string {
	actions := options.actions()
	junitAlone := options.junit != "" && !options.laws
	colourWithAction := options.color && len(actions) > 0
	switch {
	case len(actions) > 1:
		return "pass one of -laws, -index, -list, -json and -brief, not " + strings.Join(actions, " and ")
	case junitAlone:
		return "-junit goes with -laws"
	case colourWithAction:
		return "-color goes with the outline, not with " + actions[0]
	}
	return ""
}

// actions are the action flags set, in the order the usage names them.
func (options runOptions) actions() []string {
	var set []string
	if options.laws {
		set = append(set, "-laws")
	}
	if options.index != "" {
		set = append(set, "-index")
	}
	if options.list {
		set = append(set, "-list")
	}
	if options.json {
		set = append(set, "-json")
	}
	if options.brief != "" {
		set = append(set, "-brief")
	}
	return set
}

func (options runOptions) style() render.Style {
	if options.color {
		return render.ANSI
	}
	return render.Plain
}
