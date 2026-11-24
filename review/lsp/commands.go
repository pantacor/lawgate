package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/pantacor/lawgate/inventory"
	"github.com/pantacor/lawgate/review"
)

// The workspace/executeCommand commands. Their ids are a contract with the
// editor extensions (editors/vscode): rename one only with them.
const (
	CommandReview         = "lawgate.review"         // [base, lookUpRequests] → review.Report
	CommandReviewMarkdown = "lawgate.reviewMarkdown" // [base, lookUpRequests] → Markdown string
	CommandWhy            = "lawgate.why"            // [uri, line (0-based), lookUpRequests] → review.Commit
	CommandReindex        = "lawgate.reindex"        // [] → review.Summary
	CommandSummary        = "lawgate.summary"        // [] → review.Summary
	CommandIndex          = "lawgate.index"          // [] → review.Index
)

// The client commands the code lenses name; an editor extension carries
// them out (a client without them still shows the titles).
const (
	clientShowLaw     = "lawgate.showLaw"     // [law key]
	clientShowSite    = "lawgate.showSite"    // [machine, state, step, tag or step]
	clientShowState   = "lawgate.showState"   // [machine, state]
	clientShowSubject = "lawgate.showSubject" // [law key], from a subject: its tests
	clientShowTest    = "lawgate.showTest"    // [law key], from a test: its subjects
)

// Commands lists the command ids the server answers.
var Commands = []string{CommandReview, CommandReviewMarkdown, CommandWhy, CommandReindex, CommandSummary, CommandIndex}

// arguments reads a command's positional arguments.
type arguments []json.RawMessage

func (args arguments) text(position int) string {
	var value string
	if position < len(args) {
		_ = json.Unmarshal(args[position], &value)
	}
	return value
}

func (args arguments) number(position int) int {
	var value int
	if position < len(args) {
		_ = json.Unmarshal(args[position], &value)
	}
	return value
}

func (args arguments) flag(position int) bool {
	var value bool
	if position < len(args) {
		_ = json.Unmarshal(args[position], &value)
	}
	return value
}

func (s *server) executeCommand(params json.RawMessage) (any, error) {
	var p struct {
		Command   string    `json:"command"`
		Arguments arguments `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	switch p.Command {
	case CommandReview, CommandReviewMarkdown:
		return s.review(p.Command, p.Arguments)
	case CommandWhy:
		return s.why(p.Arguments)
	case CommandReindex:
		return s.reindex(), nil
	case CommandSummary:
		return s.index().Summarize(), nil
	case CommandIndex:
		return s.index(), nil
	}
	return nil, fmt.Errorf("unknown command %q (want one of %s)", p.Command, strings.Join(Commands, ", "))
}

func (s *server) review(command string, args arguments) (any, error) {
	idx := s.index()
	base := args.text(0)
	if base == "" {
		base = review.DefaultBase(s.root)
	}
	options := review.ReportOptions{MergeRequests: args.flag(1)}
	report, err := review.Review(idx, base, options)
	if err != nil {
		return nil, err
	}
	if command == CommandReviewMarkdown {
		return review.Markdown(report, ""), nil
	}
	return report, nil
}

func (s *server) why(args arguments) (any, error) {
	relative, inside := s.rel(args.text(0))
	if !inside {
		return nil, errors.New("the file is outside the repository")
	}
	commit, err := review.Blame(s.root, relative, args.number(1)+1)
	if err != nil {
		return nil, err
	}
	if args.flag(2) && !commit.Uncommitted {
		commit.MergeRequest = review.MergeRequestFor(s.root, s.platform(), commit.SHA)
	}
	return commit, nil
}

// platform is lawgate.json's platform, github by default.
func (s *server) platform() string {
	config, err := inventory.Load(s.root)
	if err != nil || config.Platform == "" {
		return "github"
	}
	return config.Platform
}
