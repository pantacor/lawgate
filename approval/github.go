package approval

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type githubEvent struct {
	PullRequest *githubPull      `json:"pull_request"`
	Ref         string           `json:"ref"`
	Repository  githubRepository `json:"repository"`
}

type githubRepository struct {
	DefaultBranch string `json:"default_branch"`
}

type githubPull struct {
	Number   int        `json:"number"`
	MergedAt string     `json:"merged_at"`
	Head     githubRef  `json:"head"`
	User     githubUser `json:"user"`
}

type githubRef struct {
	SHA string `json:"sha"`
}

type githubReview struct {
	User     githubUser `json:"user"`
	State    string     `json:"state"`
	CommitID string     `json:"commit_id"`
}

type githubUser struct {
	Login string `json:"login"`
}

// githubAPI reads pull requests and reviews of one repository.
type githubAPI struct {
	api   client
	repo  string
	owner string
}

func (judge judgement) github() (string, error) {
	vars, err := judge.env.require("GITHUB_EVENT_NAME", "GITHUB_EVENT_PATH", "GITHUB_REPOSITORY", "GITHUB_API_URL", "GITHUB_TOKEN")
	if err != nil {
		return "", err
	}
	event, err := readGitHubEvent(judge.env, vars["GITHUB_EVENT_PATH"])
	if err != nil {
		return "", err
	}
	hub, err := newGitHub(vars, judge.owner)
	if err != nil {
		return "", err
	}
	switch name := vars["GITHUB_EVENT_NAME"]; name {
	case "pull_request", "pull_request_review":
		return hub.judgePull(judge.request, event)
	case "push":
		return hub.judgePush(judge.request, event)
	case "merge_group":
		return mergeQueueLine, nil
	default:
		return "", fmt.Errorf("GITHUB_EVENT_NAME %q: the approval check runs on pull_request, pull_request_review, merge_group and push", name)
	}
}

// mergeQueueLine explains why a merge queue run reads no approval: each
// pull request was judged before it joined the queue, and the push to the
// default branch judges what lands.
const mergeQueueLine = "merge queue: each pull request was judged before it joined the queue; the push to the default branch judges what lands"

func readGitHubEvent(env Environment, path string) (githubEvent, error) {
	data, err := env.ReadFile(path)
	if err != nil {
		return githubEvent{}, fmt.Errorf("GITHUB_EVENT_PATH: %w", err)
	}
	var event githubEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return githubEvent{}, fmt.Errorf("GITHUB_EVENT_PATH: %w", err)
	}
	return event, nil
}

func newGitHub(vars map[string]string, owner string) (githubAPI, error) {
	header := http.Header{}
	header.Set("Accept", "application/vnd.github+json")
	header.Set("X-GitHub-Api-Version", "2022-11-28")
	header.Set("Authorization", "Bearer "+vars["GITHUB_TOKEN"])
	api, err := newClient(vars["GITHUB_API_URL"], "GITHUB_TOKEN", vars["GITHUB_TOKEN"], header, linkHeader)
	if err != nil {
		return githubAPI{}, fmt.Errorf("GITHUB_API_URL: %w", err)
	}
	return githubAPI{api: api, repo: vars["GITHUB_REPOSITORY"], owner: owner}, nil
}

// judgePull requires the owner's approval of the pull request at its head,
// which must be the audited HEAD.
func (hub githubAPI) judgePull(request Request, event githubEvent) (string, error) {
	pull := event.PullRequest
	incomplete := pull == nil || pull.Number == 0 || pull.Head.SHA == ""
	if incomplete {
		return "", errors.New("GITHUB_EVENT_PATH holds no pull request number and head")
	}
	if !strings.EqualFold(pull.Head.SHA, request.Head) {
		return "", fmt.Errorf("HEAD %s is not the head %s of pull request #%d: pass the pull request head as HEAD",
			shortID(request.Head), shortID(pull.Head.SHA), pull.Number)
	}
	judged, err := hub.pullVerdict(*pull)
	if err != nil {
		return "", err
	}
	if !judged.approved {
		fix := "approve the pull request at its head; the review runs the check again"
		return "", approvalMissing(request.Changes, hub.owner, pull.Head.SHA, judged, fix)
	}
	where := fmt.Sprintf("on %s in pull request #%d", shortID(pull.Head.SHA), pull.Number)
	return passLine(request.Changes, hub.owner, judged, where), nil
}

func (hub githubAPI) pullVerdict(pull githubPull) (verdict, error) {
	path := fmt.Sprintf("repos/%s/pulls/%d/reviews", hub.repo, pull.Number)
	reviews, err := getPages[githubReview](hub.api, path, nil)
	if err != nil {
		return verdict{}, err
	}
	return judgeReviews(reviews, hub.owner, pull), nil
}

// judgeReviews passes a pull request the owner opened. Otherwise it reads
// the owner's latest review that decides: it must approve the pull
// request's head. A comment decides nothing; a later request for changes
// or a dismissal wins over an approval.
func judgeReviews(reviews []githubReview, owner string, pull githubPull) verdict {
	if strings.EqualFold(pull.User.Login, owner) {
		return authoredVerdict(owner)
	}
	latest, found := latestDecision(reviews, owner)
	if !found {
		return refusedVerdict("@%s has not approved the pull request", owner)
	}
	switch latest.State {
	case "CHANGES_REQUESTED":
		return refusedVerdict("@%s requested changes", owner)
	case "DISMISSED":
		return refusedVerdict("@%s's review was dismissed", owner)
	}
	if !strings.EqualFold(latest.CommitID, pull.Head.SHA) {
		return refusedVerdict("@%s approved %s, an older commit", owner, shortID(latest.CommitID))
	}
	return approvedVerdict()
}

// decidingStates are the review states that change a pull request's
// approval; GitHub lists reviews oldest first.
var decidingStates = map[string]bool{"APPROVED": true, "CHANGES_REQUESTED": true, "DISMISSED": true}

func latestDecision(reviews []githubReview, owner string) (githubReview, bool) {
	var latest githubReview
	found := false
	for _, review := range reviews {
		ownerDecides := decidingStates[review.State] && strings.EqualFold(review.User.Login, owner)
		if ownerDecides {
			latest, found = review, true
		}
	}
	return latest, found
}

// judgePush requires, on a push to the default branch, that every
// rule-file commit came from a merged pull request the owner approved at
// its head. GitHub Free cannot block the push; this names it afterwards.
func (hub githubAPI) judgePush(request Request, event githubEvent) (string, error) {
	branch := event.Repository.DefaultBranch
	if branch == "" || event.Ref != "refs/heads/"+branch {
		return "", fmt.Errorf("push to %q: --require-approval on a push judges the default branch %q only", event.Ref, branch)
	}
	verdicts := make(map[int]verdict)
	var problems error
	for _, change := range request.Changes {
		problems = errors.Join(problems, hub.judgeCommit(change, verdicts))
	}
	if problems != nil {
		return "", problems
	}
	line := approvedLine(request.Changes, hub.owner, "in the pull requests that merged them")
	return line + authoredNotes(verdicts, "pull request #%d"), nil
}

func (hub githubAPI) judgeCommit(change Change, verdicts map[int]verdict) error {
	pulls, err := hub.mergedPulls(change.Commit)
	if err != nil {
		return err
	}
	if len(pulls) == 0 {
		return reviewMissing(change, hub.owner, "pull request")
	}
	var reasons []string
	for _, pull := range pulls {
		judged, err := hub.cachedVerdict(pull, verdicts)
		if err != nil || judged.approved {
			return err
		}
		reasons = append(reasons, fmt.Sprintf("pull request #%d: %s", pull.Number, judged.reason))
	}
	refused := refusedVerdict("%s", strings.Join(reasons, "; "))
	fix := "it is merged already: revert it, or land it again through a pull request the owner approves"
	return approvalMissing([]Change{change}, hub.owner, pulls[0].Head.SHA, refused, fix)
}

func (hub githubAPI) cachedVerdict(pull githubPull, verdicts map[int]verdict) (verdict, error) {
	if judged, known := verdicts[pull.Number]; known {
		return judged, nil
	}
	judged, err := hub.pullVerdict(pull)
	if err == nil {
		verdicts[pull.Number] = judged
	}
	return judged, err
}

// mergedPulls are the merged pull requests that brought commit in.
func (hub githubAPI) mergedPulls(commit string) ([]githubPull, error) {
	path := fmt.Sprintf("repos/%s/commits/%s/pulls", hub.repo, commit)
	pulls, err := getPages[githubPull](hub.api, path, nil)
	if err != nil {
		return nil, err
	}
	var merged []githubPull
	for _, pull := range pulls {
		if pull.MergedAt != "" {
			merged = append(merged, pull)
		}
	}
	return merged, nil
}
