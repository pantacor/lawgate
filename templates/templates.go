// Package templates holds the CI and CODEOWNERS templates used by lawgate init.
package templates

import (
	_ "embed"
	"strings"
)

//go:embed github-actions.yml
var GitHub string

//go:embed gitlab-ci.yml
var GitLab string

//go:embed github.CODEOWNERS
var githubOwners string

//go:embed gitlab.CODEOWNERS
var gitlabOwners string

// CodeownersFallback renders the fallback and platform section before
// appending a generated CODEOWNERS block. The caller validates the owner.
func CodeownersFallback(platform, owner string) string {
	content := githubOwners
	if platform == "gitlab" {
		content = gitlabOwners
	}
	return strings.ReplaceAll(content, "@your-org/law-owners", owner)
}
