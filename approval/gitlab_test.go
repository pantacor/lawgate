package approval

import (
	"fmt"
	"testing"
)

const (
	mergeRequestPath = "/api/v4/projects/42/merge_requests/5"
	pushedAt         = "2026-11-24T10:00:00.000Z"
	beforePush       = "2026-11-24T09:00:00.000Z"
	afterPush        = "2026-11-24T11:00:00.000Z"
	lateAfterPush    = "2026-11-24T12:00:00.000Z"
)

func gitlabVars(fake *fakePlatform) map[string]string {
	return map[string]string{
		"CI_MERGE_REQUEST_IID": "5",
		"CI_PROJECT_ID":        "42",
		"CI_API_V4_URL":        fake.server.URL + "/api/v4",
		"LAWGATE_API_TOKEN":    testToken,
	}
}

func approvedBy(usernames ...string) string {
	body := `{"approved_by":[`
	for index, username := range usernames {
		if index > 0 {
			body += ","
		}
		body += fmt.Sprintf(`{"user":{"username":%q}}`, username)
	}
	return body + "]}"
}

func note(username, body, created string) string {
	return fmt.Sprintf(`{"body":%q,"system":true,"author":{"username":%q},"created_at":%q}`, body, username, created)
}

func versions(head, created string) string {
	return fmt.Sprintf(`[{"head_commit_sha":%q,"created_at":%q},{"head_commit_sha":%q,"created_at":%q}]`, head, created, oldID, beforePush)
}

// recordMergeRequest records merge request !5 by @agent at headID, pushed
// at pushedAt, with the given approvals and notes (newest first).
func recordMergeRequest(fake *fakePlatform, approvals string, notes ...string) {
	recordAuthor(fake, "agent")
	fake.record(mergeRequestPath+"/versions", versions(headID, pushedAt))
	fake.record(mergeRequestPath+"/approvals", approvals)
	fake.record(mergeRequestPath+"/notes", jsonList(notes))
}

func jsonList(items []string) string {
	body := "["
	for index, item := range items {
		if index > 0 {
			body += ","
		}
		body += item
	}
	return body + "]"
}

func checkGitLab(t *testing.T, fake *fakePlatform) (string, error) {
	t.Helper()
	return Check(ruleRequest("gitlab"), testEnvironment(gitlabVars(fake), ""))
}

// TestGitLabPassesAnApprovalAfterTheLatestPush enforces rule-changes-are-reviewed.
func TestGitLabPassesAnApprovalAfterTheLatestPush(t *testing.T) {
	fake := newFakePlatform(t, nextPageHeader)
	recordMergeRequest(fake, approvedBy("highercomve"), note("highercomve", approvedNote, afterPush))
	line, err := checkGitLab(t, fake)
	assertPasses(t, line, err, "approved by @highercomve on 1a2b3c4 in merge request !5")
}

// TestGitLabRefusesAnApprovalOfAnOlderVersion enforces rule-changes-are-reviewed.
func TestGitLabRefusesAnApprovalOfAnOlderVersion(t *testing.T) {
	fake := newFakePlatform(t, nextPageHeader)
	recordMergeRequest(fake, approvedBy("highercomve"), note("highercomve", approvedNote, beforePush))
	_, err := checkGitLab(t, fake)
	assertFails(t, err, "approval by @highercomve on 1a2b3c4 is missing", "approved before the latest push", "approve the merge request and rerun the pipeline")
}

// TestGitLabRefusesUnapprovedAfterApproved enforces rule-changes-are-reviewed.
func TestGitLabRefusesUnapprovedAfterApproved(t *testing.T) {
	fake := newFakePlatform(t, nextPageHeader)
	unapproved := note("highercomve", unapprovedNote, lateAfterPush)
	recordMergeRequest(fake, approvedBy(), unapproved, note("highercomve", approvedNote, afterPush))
	_, err := checkGitLab(t, fake)
	assertFails(t, err, "@highercomve has not approved the merge request")
}

// TestGitLabTrustsTheLatestNoteOverTheApprovalList enforces rule-changes-are-reviewed.
func TestGitLabTrustsTheLatestNoteOverTheApprovalList(t *testing.T) {
	fake := newFakePlatform(t, nextPageHeader)
	unapproved := note("highercomve", unapprovedNote, lateAfterPush)
	recordMergeRequest(fake, approvedBy("highercomve"), unapproved, note("highercomve", approvedNote, afterPush))
	_, err := checkGitLab(t, fake)
	assertFails(t, err, "@highercomve revoked the approval")
}

// TestGitLabIgnoresAnotherUsersApprovalAndPlainComments enforces rule-changes-are-reviewed.
func TestGitLabIgnoresAnotherUsersApprovalAndPlainComments(t *testing.T) {
	fake := newFakePlatform(t, nextPageHeader)
	comment := fmt.Sprintf(`{"body":%q,"system":false,"author":{"username":"highercomve"},"created_at":%q}`, approvedNote, afterPush)
	recordMergeRequest(fake, approvedBy("someone", "highercomve"), comment, note("someone", approvedNote, afterPush))
	_, err := checkGitLab(t, fake)
	assertFails(t, err, "GitLab lists no approval note from @highercomve")
}

func TestGitLabFollowsNotePages(t *testing.T) {
	fake := newFakePlatform(t, nextPageHeader)
	recordMergeRequest(fake, approvedBy("highercomve"), note("someone", "changed the description", lateAfterPush))
	fake.next[mergeRequestPath+"/notes"] = "2"
	fake.record(mergeRequestPath+"/notes?page=2", jsonList([]string{note("highercomve", approvedNote, afterPush)}))
	line, err := checkGitLab(t, fake)
	assertPasses(t, line, err, "approved by @highercomve")
	if !fake.hit(mergeRequestPath + "/notes?page=2") {
		t.Fatal("the second page of notes was not read")
	}
}

func TestGitLabRefusesAHeadThatIsNotTheLatestVersion(t *testing.T) {
	fake := newFakePlatform(t, nextPageHeader)
	recordMergeRequest(fake, approvedBy("highercomve"), note("highercomve", approvedNote, afterPush))
	fake.record(mergeRequestPath+"/versions", versions(oldID, lateAfterPush))
	_, err := checkGitLab(t, fake)
	assertFails(t, err, "HEAD 1a2b3c4 is not the latest version 1111111 of merge request !5")
}

func TestGitLabFailsClosedWithoutToken(t *testing.T) {
	fake := newFakePlatform(t, nextPageHeader)
	vars := gitlabVars(fake)
	delete(vars, "LAWGATE_API_TOKEN")
	_, err := Check(ruleRequest("gitlab"), testEnvironment(vars, ""))
	assertFails(t, err, "LAWGATE_API_TOKEN is not set", "read_api")
	if len(fake.hits) != 0 {
		t.Fatalf("the check called the API without a token: %v", fake.hits)
	}
}

func TestGitLabFailsClosedWithoutPipelineContext(t *testing.T) {
	fake := newFakePlatform(t, nextPageHeader)
	vars := gitlabVars(fake)
	delete(vars, "CI_MERGE_REQUEST_IID")
	_, err := Check(ruleRequest("gitlab"), testEnvironment(vars, ""))
	assertFails(t, err, "CI_MERGE_REQUEST_IID is not set")
}

const commitMergeRequestsPath = "/api/v4/projects/42/repository/commits/" + ruleID + "/merge_requests"

func gitlabPushVars(fake *fakePlatform) map[string]string {
	vars := gitlabVars(fake)
	delete(vars, "CI_MERGE_REQUEST_IID")
	vars["CI_PIPELINE_SOURCE"] = "push"
	vars["CI_COMMIT_BRANCH"] = "main"
	vars["CI_DEFAULT_BRANCH"] = "main"
	return vars
}

func checkGitLabPush(t *testing.T, fake *fakePlatform) (string, error) {
	t.Helper()
	return Check(ruleRequest("gitlab"), testEnvironment(gitlabPushVars(fake), ""))
}

func TestGitLabPushPassesACommitFromAnApprovedMergeRequest(t *testing.T) {
	fake := newFakePlatform(t, nextPageHeader)
	fake.record(commitMergeRequestsPath, `[{"iid":3,"state":"closed"},{"iid":5,"state":"merged"}]`)
	recordMergeRequest(fake, approvedBy("highercomve"), note("highercomve", approvedNote, afterPush))
	line, err := checkGitLabPush(t, fake)
	assertPasses(t, line, err, "in the merge requests that merged them")
}

func TestGitLabPushNamesACommitWithoutMergeRequest(t *testing.T) {
	fake := newFakePlatform(t, nextPageHeader)
	fake.record(commitMergeRequestsPath, "[]")
	_, err := checkGitLabPush(t, fake)
	assertFails(t, err, "commit 2222222 reached the default branch without a merge request", "even a commit @highercomve wrote")
}

func TestGitLabPushNamesAnUnapprovedMergeRequest(t *testing.T) {
	fake := newFakePlatform(t, nextPageHeader)
	fake.record(commitMergeRequestsPath, `[{"iid":5,"state":"merged"}]`)
	recordMergeRequest(fake, approvedBy("highercomve"), note("highercomve", approvedNote, beforePush))
	_, err := checkGitLabPush(t, fake)
	assertFails(t, err, "merge request !5: @highercomve approved before the latest push", "it is merged already")
}

func TestGitLabPushJudgesTheDefaultBranchOnly(t *testing.T) {
	fake := newFakePlatform(t, nextPageHeader)
	vars := gitlabPushVars(fake)
	vars["CI_COMMIT_BRANCH"] = "topic"
	_, err := Check(ruleRequest("gitlab"), testEnvironment(vars, ""))
	assertFails(t, err, `push to "topic"`)
}

func recordAuthor(fake *fakePlatform, author string) {
	fake.record(mergeRequestPath, fmt.Sprintf(`{"iid":5,"state":"opened","author":{"username":%q}}`, author))
}

// TestGitLabPassesAMergeRequestTheOwnerAuthored enforces rule-changes-are-reviewed.
func TestGitLabPassesAMergeRequestTheOwnerAuthored(t *testing.T) {
	fake := newFakePlatform(t, nextPageHeader)
	recordMergeRequest(fake, approvedBy())
	recordAuthor(fake, "HigherComve")
	line, err := checkGitLab(t, fake)
	assertPasses(t, line, err, "authored by @highercomve: counts as approved, on 1a2b3c4 in merge request !5")
}

// TestGitLabMergeRequestByAnotherAuthorNeedsTheApproval enforces rule-changes-are-reviewed.
func TestGitLabMergeRequestByAnotherAuthorNeedsTheApproval(t *testing.T) {
	fake := newFakePlatform(t, nextPageHeader)
	recordMergeRequest(fake, approvedBy())
	_, err := checkGitLab(t, fake)
	assertFails(t, err, "@highercomve has not approved the merge request")
}

func TestGitLabPushPassesACommitFromAMergeRequestTheOwnerAuthored(t *testing.T) {
	fake := newFakePlatform(t, nextPageHeader)
	fake.record(commitMergeRequestsPath, `[{"iid":5,"state":"merged","author":{"username":"highercomve"}}]`)
	recordMergeRequest(fake, approvedBy())
	recordAuthor(fake, "highercomve")
	line, err := checkGitLabPush(t, fake)
	assertPasses(t, line, err, "merge request !5 authored by @highercomve: counts as approved")
}
