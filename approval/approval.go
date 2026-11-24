// Package approval checks, in CI, that the owner approved the pull request
// or merge request behind every change to a rule file. It asks the
// platform's API, because required code-owner review, branch protection and
// required checks are paid features on the free plans of GitHub and GitLab
// (owner decision 2026-11-24). It fails closed: outside CI, without a token
// or without the platform's answer, the check fails.
package approval

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Change is a commit that changed rule files.
type Change struct {
	Commit string
	Files  []string
}

// Request is what Check judges.
type Request struct {
	// Owner is lawgate.json's owner, a user such as @name.
	Owner string
	// Platform is lawgate.json's platform: github or gitlab.
	Platform string
	// Head is the commit ID of the audited HEAD.
	Head string
	// Changes are the commits of the audited range that changed rule files.
	Changes []Change
}

// Environment is where Check reads the CI context.
type Environment struct {
	Getenv   func(string) string
	ReadFile func(string) ([]byte, error)
}

// OS reads the process environment and the file system.
func OS() Environment {
	return Environment{Getenv: os.Getenv, ReadFile: os.ReadFile}
}

// Check returns a line that says why the change passes, or an error that
// says which approval is missing and how to get it.
func Check(request Request, env Environment) (string, error) {
	if len(request.Changes) == 0 {
		return "no rule files changed: no approval needed", nil
	}
	owner, err := ownerLogin(request.Owner)
	if err != nil {
		return "", err
	}
	judge := judgement{owner: owner, request: request, env: env}
	switch request.Platform {
	case "github":
		return judge.github()
	case "gitlab":
		return judge.gitlab()
	}
	return "", fmt.Errorf("platform %q: --require-approval reads github or gitlab", request.Platform)
}

// judgement carries one Check call to the platform that answers it.
type judgement struct {
	owner   string
	request Request
	env     Environment
}

// ownerLogin is the owner's user name without the @. A team or group cannot
// approve a merge request or submit a review, so it is refused.
func ownerLogin(owner string) (string, error) {
	login, handle := strings.CutPrefix(owner, "@")
	if !handle || login == "" {
		return "", fmt.Errorf("owner %q: --require-approval needs a user such as @name in lawgate.json", owner)
	}
	if strings.Contains(login, "/") {
		return "", fmt.Errorf("owner %s is a team or group: only a user can approve a rule change here; set lawgate.json owner to the user who approves rule changes", owner)
	}
	return login, nil
}

// verdict is the owner's approval on one pull request or merge request:
// approved, or the reason it is not. An approval by authorship carries
// its reason too, so the verdict line can say so.
type verdict struct {
	approved bool
	reason   string
}

func approvedVerdict() verdict {
	return verdict{approved: true}
}

// authoredVerdict passes a pull request or merge request the owner opened:
// GitHub does not let an author approve their own pull request, and the
// owner decided (2026-11-24) that authorship counts as approval. Work pushed
// with the owner's credentials, an agent's included, looks authored by the
// owner; separate accounts close that.
func authoredVerdict(owner string) verdict {
	return verdict{approved: true, reason: fmt.Sprintf("authored by @%s: counts as approved", owner)}
}

func refusedVerdict(format string, values ...any) verdict {
	return verdict{reason: fmt.Sprintf(format, values...)}
}

// variableHints say how to provide a CI variable a job did not pass.
var variableHints = map[string]string{
	"GITHUB_TOKEN":      "pass GITHUB_TOKEN: ${{ github.token }} in the step's env, with permissions pull-requests: read",
	"LAWGATE_API_TOKEN": "add a masked CI/CD variable LAWGATE_API_TOKEN holding a token with the read_api scope",
}

// require reads every named variable, and names each one that is empty.
func (env Environment) require(names ...string) (map[string]string, error) {
	values := make(map[string]string, len(names))
	var missing []string
	for _, name := range names {
		value := env.Getenv(name)
		if value == "" {
			missing = append(missing, name)
		}
		values[name] = value
	}
	if len(missing) == 0 {
		return values, nil
	}
	return nil, missingVariables(missing)
}

func missingVariables(names []string) error {
	problems := []error{errors.New("cannot check the owner's approval: --require-approval runs in GitHub Actions or GitLab CI and never passes without the platform's answer")}
	for _, name := range names {
		hint := variableHints[name]
		if hint == "" {
			hint = "the CI platform sets it; run the check in the CI job"
		}
		problems = append(problems, fmt.Errorf("%s is not set: %s", name, hint))
	}
	return errors.Join(problems...)
}
