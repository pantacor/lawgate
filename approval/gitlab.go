package approval

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type gitlabUser struct {
	Username string `json:"username"`
}

type gitlabApprovals struct {
	ApprovedBy []gitlabApprover `json:"approved_by"`
}

type gitlabApprover struct {
	User gitlabUser `json:"user"`
}

type gitlabNote struct {
	Body      string     `json:"body"`
	System    bool       `json:"system"`
	Author    gitlabUser `json:"author"`
	CreatedAt time.Time  `json:"created_at"`
}

type gitlabVersion struct {
	HeadCommitSHA string    `json:"head_commit_sha"`
	CreatedAt     time.Time `json:"created_at"`
}

type gitlabMergeRequest struct {
	IID    int        `json:"iid"`
	State  string     `json:"state"`
	Author gitlabUser `json:"author"`
}

// The system notes GitLab writes when a user approves or revokes an
// approval.
const (
	approvedNote   = "approved this merge request"
	unapprovedNote = "unapproved this merge request"
)

// gitlabAPI reads merge requests of one project.
type gitlabAPI struct {
	api     client
	project string
	owner   string
}

func (judge judgement) gitlab() (string, error) {
	vars, err := judge.env.require("CI_PROJECT_ID", "CI_API_V4_URL", "LAWGATE_API_TOKEN")
	if err != nil {
		return "", err
	}
	lab, err := newGitLab(vars, judge.owner)
	if err != nil {
		return "", err
	}
	if iid := judge.env.Getenv("CI_MERGE_REQUEST_IID"); iid != "" {
		return lab.judgeMergeRequest(judge.request, iid)
	}
	if judge.env.Getenv("CI_PIPELINE_SOURCE") == "push" {
		return lab.judgePush(judge.request, judge.env)
	}
	return "", errors.New("CI_MERGE_REQUEST_IID is not set and CI_PIPELINE_SOURCE is not push: --require-approval runs in a merge request pipeline or a push pipeline on the default branch")
}

// newGitLab authenticates with LAWGATE_API_TOKEN: CI_JOB_TOKEN cannot read
// a merge request's approvals, notes or versions.
func newGitLab(vars map[string]string, owner string) (gitlabAPI, error) {
	header := http.Header{}
	header.Set("PRIVATE-TOKEN", vars["LAWGATE_API_TOKEN"])
	api, err := newClient(vars["CI_API_V4_URL"], "LAWGATE_API_TOKEN", vars["LAWGATE_API_TOKEN"], header, nextPageHeader)
	if err != nil {
		return gitlabAPI{}, fmt.Errorf("CI_API_V4_URL: %w", err)
	}
	return gitlabAPI{api: api, project: vars["CI_PROJECT_ID"], owner: owner}, nil
}

// judgeMergeRequest requires the owner's approval of the merge request's
// latest version, which must be the audited HEAD.
func (lab gitlabAPI) judgeMergeRequest(request Request, iid string) (string, error) {
	state, err := lab.mergeRequestState(iid)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(state.head, request.Head) {
		return "", fmt.Errorf("HEAD %s is not the latest version %s of merge request !%s: rerun the pipeline of the latest push",
			shortID(request.Head), shortID(state.head), iid)
	}
	if !state.verdict.approved {
		fix := "approve the merge request and rerun the pipeline"
		return "", approvalMissing(request.Changes, lab.owner, state.head, state.verdict, fix)
	}
	where := fmt.Sprintf("on %s in merge request !%s", shortID(state.head), iid)
	return passLine(request.Changes, lab.owner, state.verdict, where), nil
}

// mergeRequestState is a merge request's latest version and the owner's
// approval of it.
type mergeRequestState struct {
	head    string
	verdict verdict
}

func (lab gitlabAPI) mergeRequestState(iid string) (mergeRequestState, error) {
	path := fmt.Sprintf("projects/%s/merge_requests/%s", url.PathEscape(lab.project), url.PathEscape(iid))
	var merge gitlabMergeRequest
	if err := lab.api.getOne(path, &merge); err != nil {
		return mergeRequestState{}, err
	}
	latest, err := lab.latestVersion(path, iid)
	if err != nil {
		return mergeRequestState{}, err
	}
	var approvals gitlabApprovals
	if err := lab.api.getOne(path+"/approvals", &approvals); err != nil {
		return mergeRequestState{}, err
	}
	note, err := lab.latestApprovalNote(path)
	if err != nil {
		return mergeRequestState{}, err
	}
	evidence := gitlabEvidence{author: merge.Author.Username, approvals: approvals, note: note, version: latest}
	return mergeRequestState{head: latest.HeadCommitSHA, verdict: evidence.verdict(lab.owner)}, nil
}

// latestVersion is the newest diff version: GitLab adds one on every push.
func (lab gitlabAPI) latestVersion(path, iid string) (gitlabVersion, error) {
	versions, err := getPages[gitlabVersion](lab.api, path+"/versions", nil)
	if err != nil {
		return gitlabVersion{}, err
	}
	if len(versions) == 0 {
		return gitlabVersion{}, fmt.Errorf("merge request !%s has no diff version yet: rerun the pipeline", iid)
	}
	latest := versions[0]
	for _, version := range versions[1:] {
		if version.CreatedAt.After(latest.CreatedAt) {
			latest = version
		}
	}
	return latest, nil
}

// latestApprovalNote is the owner's newest approve or unapprove system
// note, or nil. Notes come newest first, so the listing stops there.
func (lab gitlabAPI) latestApprovalNote(path string) (*gitlabNote, error) {
	query := url.Values{}
	query.Set("sort", "desc")
	query.Set("order_by", "created_at")
	var found *gitlabNote
	err := eachPage(lab.api, path+"/notes", query, func(notes []gitlabNote) bool {
		found = lab.firstApprovalNote(notes)
		return found == nil
	})
	return found, err
}

func (lab gitlabAPI) firstApprovalNote(notes []gitlabNote) *gitlabNote {
	for index := range notes {
		note := &notes[index]
		approval := note.System && strings.EqualFold(note.Author.Username, lab.owner)
		if approval && approvalNoteKind(note.Body) != "" {
			return note
		}
	}
	return nil
}

// approvalNoteKind is approvedNote, unapprovedNote or "" for any other note.
func approvalNoteKind(body string) string {
	for _, kind := range []string{approvedNote, unapprovedNote} {
		if strings.HasPrefix(strings.TrimSpace(body), kind) {
			return kind
		}
	}
	return ""
}

// gitlabEvidence is what GitLab tells about the owner's approval. A merge
// request the owner authored passes. GitLab Free keeps an approval when new
// commits are pushed, so an approval counts only when it is newer than the
// latest diff version.
type gitlabEvidence struct {
	author    string
	approvals gitlabApprovals
	note      *gitlabNote
	version   gitlabVersion
}

func (evidence gitlabEvidence) verdict(owner string) verdict {
	if strings.EqualFold(evidence.author, owner) {
		return authoredVerdict(owner)
	}
	if !evidence.approvedBy(owner) {
		return refusedVerdict("@%s has not approved the merge request", owner)
	}
	if evidence.note == nil {
		return refusedVerdict("GitLab lists no approval note from @%s", owner)
	}
	if approvalNoteKind(evidence.note.Body) != approvedNote {
		return refusedVerdict("@%s revoked the approval", owner)
	}
	if !evidence.note.CreatedAt.After(evidence.version.CreatedAt) {
		return refusedVerdict("@%s approved before the latest push (%s)", owner, shortID(evidence.version.HeadCommitSHA))
	}
	return approvedVerdict()
}

func (evidence gitlabEvidence) approvedBy(owner string) bool {
	for _, approver := range evidence.approvals.ApprovedBy {
		if strings.EqualFold(approver.User.Username, owner) {
			return true
		}
	}
	return false
}

// judgePush requires, on a push pipeline of the default branch, that every
// rule-file commit came from a merged merge request the owner approved at
// its last version.
func (lab gitlabAPI) judgePush(request Request, env Environment) (string, error) {
	vars, err := env.require("CI_COMMIT_BRANCH", "CI_DEFAULT_BRANCH")
	if err != nil {
		return "", err
	}
	if vars["CI_COMMIT_BRANCH"] != vars["CI_DEFAULT_BRANCH"] {
		return "", fmt.Errorf("push to %q: --require-approval on a push judges the default branch %q only", vars["CI_COMMIT_BRANCH"], vars["CI_DEFAULT_BRANCH"])
	}
	verdicts := make(map[int]verdict)
	var problems error
	for _, change := range request.Changes {
		problems = errors.Join(problems, lab.judgeCommit(change, verdicts))
	}
	if problems != nil {
		return "", problems
	}
	line := approvedLine(request.Changes, lab.owner, "in the merge requests that merged them")
	return line + authoredNotes(verdicts, "merge request !%d"), nil
}

func (lab gitlabAPI) judgeCommit(change Change, verdicts map[int]verdict) error {
	requests, err := lab.mergedRequests(change.Commit)
	if err != nil {
		return err
	}
	if len(requests) == 0 {
		return reviewMissing(change, lab.owner, "merge request")
	}
	var reasons []string
	for _, merged := range requests {
		judged, err := lab.cachedVerdict(merged.IID, verdicts)
		if err != nil || judged.approved {
			return err
		}
		reasons = append(reasons, fmt.Sprintf("merge request !%d: %s", merged.IID, judged.reason))
	}
	refused := refusedVerdict("%s", strings.Join(reasons, "; "))
	fix := "it is merged already: revert it, or land it again through a merge request the owner approves"
	return approvalMissing([]Change{change}, lab.owner, change.Commit, refused, fix)
}

func (lab gitlabAPI) cachedVerdict(iid int, verdicts map[int]verdict) (verdict, error) {
	if judged, known := verdicts[iid]; known {
		return judged, nil
	}
	state, err := lab.mergeRequestState(strconv.Itoa(iid))
	if err == nil {
		verdicts[iid] = state.verdict
	}
	return state.verdict, err
}

// mergedRequests are the merged merge requests that brought commit in.
func (lab gitlabAPI) mergedRequests(commit string) ([]gitlabMergeRequest, error) {
	path := fmt.Sprintf("projects/%s/repository/commits/%s/merge_requests", url.PathEscape(lab.project), commit)
	requests, err := getPages[gitlabMergeRequest](lab.api, path, nil)
	if err != nil {
		return nil, err
	}
	var merged []gitlabMergeRequest
	for _, request := range requests {
		if request.State == "merged" {
			merged = append(merged, request)
		}
	}
	return merged, nil
}
