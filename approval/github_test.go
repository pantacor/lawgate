package approval

import (
	"fmt"
	"testing"
)

const reviewsPath = "/repos/pantacor/lawgate/pulls/7/reviews"

func githubVars(fake *fakePlatform, event string) map[string]string {
	return map[string]string{
		"GITHUB_EVENT_NAME": event,
		"GITHUB_EVENT_PATH": "/event.json",
		"GITHUB_REPOSITORY": "pantacor/lawgate",
		"GITHUB_API_URL":    fake.server.URL,
		"GITHUB_TOKEN":      testToken,
	}
}

func pullEvent(head string) string {
	return pullEventBy("agent", head)
}

func pullEventBy(author, head string) string {
	return fmt.Sprintf(`{"pull_request":{"number":7,"user":{"login":%q},"head":{"sha":%q}}}`, author, head)
}

func mergedPull(author string) string {
	return fmt.Sprintf(`[{"number":7,"merged_at":"2026-11-24T10:00:00Z","user":{"login":%q},"head":{"sha":%q}}]`, author, headID)
}

func review(login, state, commit string) string {
	return fmt.Sprintf(`{"user":{"login":%q},"state":%q,"commit_id":%q}`, login, state, commit)
}

func checkGitHubPull(t *testing.T, fake *fakePlatform) (string, error) {
	t.Helper()
	env := testEnvironment(githubVars(fake, "pull_request_review"), pullEvent(headID))
	return Check(ruleRequest("github"), env)
}

// TestGitHubPassesTheOwnersApprovalOfTheHead enforces rule-changes-are-reviewed.
func TestGitHubPassesTheOwnersApprovalOfTheHead(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	fake.record(reviewsPath, "["+review("highercomve", "COMMENTED", oldID)+","+review("HigherComve", "APPROVED", headID)+"]")
	line, err := checkGitHubPull(t, fake)
	assertPasses(t, line, err, "rule files changed (LAWS.lock, LAWS.md): approved by @highercomve on 1a2b3c4 in pull request #7")
}

// TestGitHubRefusesAnApprovalOfAnOlderCommit enforces rule-changes-are-reviewed.
func TestGitHubRefusesAnApprovalOfAnOlderCommit(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	fake.record(reviewsPath, "["+review("highercomve", "APPROVED", oldID)+"]")
	_, err := checkGitHubPull(t, fake)
	assertFails(t, err, "approval by @highercomve on 1a2b3c4 is missing", "approved 1111111, an older commit", "approve the pull request")
}

// TestGitHubRefusesChangesRequestedAfterApproval enforces rule-changes-are-reviewed.
func TestGitHubRefusesChangesRequestedAfterApproval(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	fake.record(reviewsPath, "["+review("highercomve", "APPROVED", headID)+","+review("highercomve", "CHANGES_REQUESTED", headID)+"]")
	_, err := checkGitHubPull(t, fake)
	assertFails(t, err, "requested changes")
}

// TestGitHubRefusesADismissedApproval enforces rule-changes-are-reviewed.
func TestGitHubRefusesADismissedApproval(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	fake.record(reviewsPath, "["+review("highercomve", "DISMISSED", headID)+"]")
	_, err := checkGitHubPull(t, fake)
	assertFails(t, err, "review was dismissed")
}

func TestGitHubRefusesAnotherUsersApproval(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	fake.record(reviewsPath, "["+review("someone", "APPROVED", headID)+"]")
	_, err := checkGitHubPull(t, fake)
	assertFails(t, err, "@highercomve has not approved the pull request")
}

func TestGitHubFollowsReviewPages(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	fake.record(reviewsPath, "["+review("someone", "APPROVED", headID)+"]")
	fake.next[reviewsPath] = "2"
	fake.record(reviewsPath+"?page=2", "["+review("highercomve", "APPROVED", headID)+"]")
	line, err := checkGitHubPull(t, fake)
	assertPasses(t, line, err, "approved by @highercomve")
	if !fake.hit(reviewsPath + "?page=2") {
		t.Fatal("the second page of reviews was not read")
	}
}

func TestGitHubRefusesAHeadThatIsNotThePullRequests(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	env := testEnvironment(githubVars(fake, "pull_request"), pullEvent(oldID))
	_, err := Check(ruleRequest("github"), env)
	assertFails(t, err, "HEAD 1a2b3c4 is not the head 1111111 of pull request #7")
}

func TestGitHubFailsClosedWithoutToken(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	vars := githubVars(fake, "pull_request")
	delete(vars, "GITHUB_TOKEN")
	_, err := Check(ruleRequest("github"), testEnvironment(vars, pullEvent(headID)))
	assertFails(t, err, "GITHUB_TOKEN is not set", "pull-requests: read")
	if len(fake.hits) != 0 {
		t.Fatalf("the check called the API without a token: %v", fake.hits)
	}
}

func TestGitHubFailsClosedOutsideCI(t *testing.T) {
	_, err := Check(ruleRequest("github"), testEnvironment(map[string]string{}, ""))
	assertFails(t, err, "GITHUB_EVENT_NAME is not set", "GITHUB_REPOSITORY is not set")
}

func TestGitHubRefusesAnUnsupportedEvent(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	env := testEnvironment(githubVars(fake, "workflow_dispatch"), "{}")
	_, err := Check(ruleRequest("github"), env)
	assertFails(t, err, `GITHUB_EVENT_NAME "workflow_dispatch"`)
}

func TestGitHubMergeQueueLeavesTheJudgementToThePush(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	env := testEnvironment(githubVars(fake, "merge_group"), "{}")
	line, err := Check(ruleRequest("github"), env)
	assertPasses(t, line, err, "the push to the default branch judges what lands")
}

func TestGitHubRejectedTokenIsNotShown(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	vars := githubVars(fake, "pull_request")
	vars["GITHUB_TOKEN"] = "wrong-" + testToken
	_, err := Check(ruleRequest("github"), testEnvironment(vars, pullEvent(headID)))
	assertFails(t, err, "401 Unauthorized", "Bad credentials", "check that GITHUB_TOKEN can read")
}

const commitPullsPath = "/repos/pantacor/lawgate/commits/" + ruleID + "/pulls"

func pushEvent() string {
	return `{"ref":"refs/heads/main","repository":{"default_branch":"main"}}`
}

func checkGitHubPush(t *testing.T, fake *fakePlatform) (string, error) {
	t.Helper()
	env := testEnvironment(githubVars(fake, "push"), pushEvent())
	return Check(ruleRequest("github"), env)
}

// TestGitHubPushPassesACommitFromAnApprovedPullRequest enforces rule-changes-are-reviewed.
func TestGitHubPushPassesACommitFromAnApprovedPullRequest(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	fake.record(commitPullsPath, mergedPull("agent"))
	fake.record(reviewsPath, "["+review("highercomve", "APPROVED", headID)+"]")
	line, err := checkGitHubPush(t, fake)
	assertPasses(t, line, err, "in the pull requests that merged them")
}

// TestGitHubPushNamesACommitWithoutPullRequest enforces rule-changes-are-reviewed.
func TestGitHubPushNamesACommitWithoutPullRequest(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	fake.record(commitPullsPath, "[]")
	_, err := checkGitHubPush(t, fake)
	assertFails(t, err, "commit 2222222 reached the default branch without a pull request", "even a commit @highercomve wrote")
}

// TestGitHubPushNamesAnUnapprovedPullRequest enforces rule-changes-are-reviewed.
func TestGitHubPushNamesAnUnapprovedPullRequest(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	fake.record(commitPullsPath, mergedPull("agent"))
	fake.record(reviewsPath, "["+review("highercomve", "APPROVED", oldID)+"]")
	_, err := checkGitHubPush(t, fake)
	assertFails(t, err, "pull request #7: @highercomve approved 1111111", "it is merged already")
}

func TestGitHubPushIgnoresAnOpenPullRequest(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	fake.record(commitPullsPath, fmt.Sprintf(`[{"number":7,"merged_at":null,"head":{"sha":%q}}]`, headID))
	fake.record(reviewsPath, "["+review("highercomve", "APPROVED", headID)+"]")
	_, err := checkGitHubPush(t, fake)
	assertFails(t, err, "without a pull request")
}

func TestGitHubPushJudgesTheDefaultBranchOnly(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	event := `{"ref":"refs/heads/topic","repository":{"default_branch":"main"}}`
	_, err := Check(ruleRequest("github"), testEnvironment(githubVars(fake, "push"), event))
	assertFails(t, err, `push to "refs/heads/topic"`)
}

// TestGitHubPassesAPullRequestTheOwnerAuthored enforces rule-changes-are-reviewed.
func TestGitHubPassesAPullRequestTheOwnerAuthored(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	fake.record(reviewsPath, "[]")
	env := testEnvironment(githubVars(fake, "pull_request"), pullEventBy("HigherComve", headID))
	line, err := Check(ruleRequest("github"), env)
	assertPasses(t, line, err, "authored by @highercomve: counts as approved, on 1a2b3c4 in pull request #7")
}

// TestGitHubPullRequestByAnotherAuthorNeedsTheApproval enforces rule-changes-are-reviewed.
func TestGitHubPullRequestByAnotherAuthorNeedsTheApproval(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	fake.record(reviewsPath, "[]")
	env := testEnvironment(githubVars(fake, "pull_request"), pullEventBy("agent", headID))
	_, err := Check(ruleRequest("github"), env)
	assertFails(t, err, "@highercomve has not approved the pull request")
}

// TestGitHubPushPassesACommitFromAPullRequestTheOwnerAuthored enforces rule-changes-are-reviewed.
func TestGitHubPushPassesACommitFromAPullRequestTheOwnerAuthored(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	fake.record(commitPullsPath, mergedPull("highercomve"))
	fake.record(reviewsPath, "[]")
	line, err := checkGitHubPush(t, fake)
	assertPasses(t, line, err, "pull request #7 authored by @highercomve: counts as approved")
}

// TestGitHubPushReportsAFailedPullRequestLookup enforces rule-changes-are-reviewed.
func TestGitHubPushReportsAFailedPullRequestLookup(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	_, err := checkGitHubPush(t, fake)
	assertFails(t, err, "commits/"+ruleID+"/pulls: 404")
}

// TestGitHubPushReportsAFailedReviewLookup enforces rule-changes-are-reviewed.
func TestGitHubPushReportsAFailedReviewLookup(t *testing.T) {
	fake := newFakePlatform(t, linkHeader)
	fake.record(commitPullsPath, mergedPull("agent"))
	_, err := checkGitHubPush(t, fake)
	assertFails(t, err, "pulls/7/reviews: 404")
}
