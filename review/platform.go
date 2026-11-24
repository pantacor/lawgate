package review

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// lookupTimeout bounds one pull or merge request lookup: it is a nicety, and
// must not hold up a review.
const lookupTimeout = 5 * time.Second

// lookupCommands are the platform CLIs that tell which pull or merge request
// brought a commit, by lawgate.json platform. "{sha}" is replaced.
var lookupCommands = map[string][]string{
	"github": {"gh", "api", "repos/{owner}/{repo}/commits/{sha}/pulls", "--jq", ".[0].html_url"},
	"gitlab": {"glab", "api", "projects/:id/repository/commits/{sha}/merge_requests"},
}

var mergeRequestURL = regexp.MustCompile(`"web_url":"([^"]+/merge_requests/\d+)"`)

// MergeRequestFor asks the platform's CLI (gh for github, glab for gitlab)
// which pull or merge request brought a commit. It returns "" when the CLI
// is not installed, not logged in, or cannot tell: the lookup is never
// required.
func MergeRequestFor(root, platform, sha string) string {
	template, known := lookupCommands[platform]
	if !known {
		return ""
	}
	if _, err := exec.LookPath(template[0]); err != nil {
		return ""
	}
	args := make([]string, 0, len(template))
	for _, arg := range template {
		args = append(args, strings.ReplaceAll(arg, "{sha}", sha))
	}
	ctx, cancel := context.WithTimeout(context.Background(), lookupTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	command.Dir = root
	out, err := command.Output()
	if err != nil {
		return ""
	}
	return requestURL(platform, out)
}

func requestURL(platform string, out []byte) string {
	if platform == "github" {
		return strings.TrimSpace(string(out))
	}
	match := mergeRequestURL.FindSubmatch(out)
	if match == nil {
		return ""
	}
	return string(match[1])
}
