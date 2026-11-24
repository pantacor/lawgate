package approval

import (
	"net/http"
	"net/url"
	"testing"
)

func TestNoRuleFilesNeedNoApproval(t *testing.T) {
	request := Request{Owner: "@highercomve", Platform: "github", Head: headID}
	line, err := Check(request, testEnvironment(map[string]string{}, ""))
	assertPasses(t, line, err, "no rule files changed: no approval needed")
}

func TestTeamOwnerIsRefused(t *testing.T) {
	for _, owner := range []string{"@pantacor/law-owners", "@group/sub/team"} {
		request := ruleRequest("gitlab")
		request.Owner = owner
		_, err := Check(request, testEnvironment(map[string]string{}, ""))
		assertFails(t, err, "is a team or group", "only a user can approve")
	}
}

func TestOwnerWithoutHandleIsRefused(t *testing.T) {
	request := ruleRequest("github")
	request.Owner = "highercomve"
	_, err := Check(request, testEnvironment(map[string]string{}, ""))
	assertFails(t, err, "needs a user such as @name")
}

func TestUnknownPlatformIsRefused(t *testing.T) {
	_, err := Check(ruleRequest("bitbucket"), testEnvironment(map[string]string{}, ""))
	assertFails(t, err, `platform "bitbucket"`)
}

func TestManyRuleFilesAreCounted(t *testing.T) {
	change := Change{Commit: ruleID, Files: []string{"a", "b", "c", "d", "e", "f", "g", "h", "a"}}
	if got := ruleFiles([]Change{change}); got != "a, b, c, d, e, f and 2 more" {
		t.Fatalf("ruleFiles = %q", got)
	}
}

func testClient(t *testing.T, pages pagination) client {
	t.Helper()
	api, err := newClient("https://api.example.test/api/v4/", "TOKEN", testToken, http.Header{}, pages)
	if err != nil {
		t.Fatal(err)
	}
	return api
}

func emptyResponse() *http.Response {
	header := http.Header{}
	return &http.Response{Header: header}
}

func TestNextLinkStaysOnTheAPIHost(t *testing.T) {
	api := testClient(t, linkHeader)
	current, _ := url.Parse("https://api.example.test/repos/o/r/pulls/1/reviews")
	response := emptyResponse()
	response.Header.Set("Link", `<https://elsewhere.test/page2>; rel="next"`)
	if _, err := api.nextPage(response, current); err == nil {
		t.Fatal("a next page on another host must be refused: it would receive the token")
	}
	response.Header.Set("Link", `<https://api.example.test/page2>; rel="next"`)
	next, err := api.nextPage(response, current)
	if err != nil || next.String() != "https://api.example.test/page2" {
		t.Fatalf("next = %v, %v", next, err)
	}
}

func TestNextPageHeaderSetsThePage(t *testing.T) {
	api := testClient(t, nextPageHeader)
	current, _ := url.Parse("https://api.example.test/api/v4/x?per_page=100&sort=desc")
	response := emptyResponse()
	response.Header.Set("X-Next-Page", "3")
	next, err := api.nextPage(response, current)
	if err != nil {
		t.Fatal(err)
	}
	keepsQuery := next.Query().Get("page") == "3" && next.Query().Get("sort") == "desc"
	if !keepsQuery {
		t.Fatalf("next = %v, %v", next, err)
	}
	response.Header.Set("X-Next-Page", "-1")
	if _, err := api.nextPage(response, current); err == nil {
		t.Fatal("an invalid X-Next-Page must fail")
	}
}

func TestEndlessPagesStop(t *testing.T) {
	fake := newFakePlatform(t, nextPageHeader)
	fake.record("/api/v4/items", "[]")
	fake.next["/api/v4/items"] = "1"
	fake.record("/api/v4/items?page=1", "[]")
	fake.next["/api/v4/items?page=1"] = "1"
	api, err := newClient(fake.server.URL+"/api/v4", "LAWGATE_API_TOKEN", testToken, gitlabHeader(), nextPageHeader)
	if err != nil {
		t.Fatal(err)
	}
	_, err = getPages[gitlabNote](api, "items", nil)
	assertFails(t, err, "more than 100 pages")
}

func TestRedirectToAnotherHostIsRefused(t *testing.T) {
	fake := newFakePlatform(t, nextPageHeader)
	fake.server.Config.Handler = http.RedirectHandler("https://elsewhere.test/steal", http.StatusFound)
	api, err := newClient(fake.server.URL, "LAWGATE_API_TOKEN", testToken, gitlabHeader(), nextPageHeader)
	if err != nil {
		t.Fatal(err)
	}
	var items []gitlabNote
	err = api.getOne("items", &items)
	assertFails(t, err, "leaves the API host")
}

func gitlabHeader() http.Header {
	header := http.Header{}
	header.Set("PRIVATE-TOKEN", testToken)
	return header
}

func TestRelativeAPIURLIsRefused(t *testing.T) {
	if _, err := newClient("api.github.com", "GITHUB_TOKEN", testToken, http.Header{}, linkHeader); err == nil {
		t.Fatal("a URL without scheme and host must be refused")
	}
}
