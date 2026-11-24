package approval

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testToken = "secret-test-token"
	headID    = "1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b"
	oldID     = "1111111111111111111111111111111111111111"
	ruleID    = "2222222222222222222222222222222222222222"
)

// fakePlatform answers API paths with recorded JSON bodies. A body for page
// N of a path is recorded under "path?page=N"; next names the page that
// follows a recorded key.
type fakePlatform struct {
	t      *testing.T
	style  pagination
	bodies map[string]string
	next   map[string]string
	server *httptest.Server
	hits   []string
}

func newFakePlatform(t *testing.T, style pagination) *fakePlatform {
	t.Helper()
	fake := &fakePlatform{t: t, style: style}
	fake.bodies = make(map[string]string)
	fake.next = make(map[string]string)
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(fake.server.Close)
	return fake
}

func (fake *fakePlatform) serve(response http.ResponseWriter, request *http.Request) {
	key := request.URL.Path
	if page := request.URL.Query().Get("page"); page != "" {
		key += "?page=" + page
	}
	fake.hits = append(fake.hits, key)
	if !fake.authorized(request) {
		http.Error(response, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
		return
	}
	body, recorded := fake.bodies[key]
	if !recorded {
		http.Error(response, `{"message":"404 Not Found"}`, http.StatusNotFound)
		return
	}
	fake.setNext(response, request, key)
	response.Header().Set("Content-Type", "application/json")
	response.Write([]byte(body))
}

func (fake *fakePlatform) authorized(request *http.Request) bool {
	if fake.style == nextPageHeader {
		return request.Header.Get("PRIVATE-TOKEN") == testToken
	}
	return request.Header.Get("Authorization") == "Bearer "+testToken
}

func (fake *fakePlatform) setNext(response http.ResponseWriter, request *http.Request, key string) {
	page, more := fake.next[key]
	if !more {
		return
	}
	if fake.style == nextPageHeader {
		response.Header().Set("X-Next-Page", page)
		return
	}
	link := fake.server.URL + request.URL.Path + "?per_page=100&page=" + page
	response.Header().Set("Link", "<"+link+`>; rel="next", <`+link+`>; rel="last"`)
}

func (fake *fakePlatform) record(key, body string) {
	fake.bodies[key] = body
}

func (fake *fakePlatform) hit(key string) bool {
	for _, hit := range fake.hits {
		if hit == key {
			return true
		}
	}
	return false
}

// testEnvironment reads variables from vars and serves event as the GitHub
// event file.
func testEnvironment(vars map[string]string, event string) Environment {
	return Environment{
		Getenv:   func(name string) string { return vars[name] },
		ReadFile: func(string) ([]byte, error) { return []byte(event), nil },
	}
}

func ruleRequest(platform string) Request {
	change := Change{Commit: ruleID, Files: []string{"LAWS.md", "LAWS.lock"}}
	return Request{Owner: "@highercomve", Platform: platform, Head: headID, Changes: []Change{change}}
}

func assertFails(t *testing.T, err error, wants ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("the check passed; want a failure naming %q", wants)
	}
	for _, want := range wants {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error does not say %q:\n%v", want, err)
		}
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("error shows the token:\n%v", err)
	}
}

func assertPasses(t *testing.T, line string, err error, want string) {
	t.Helper()
	if err != nil {
		t.Fatalf("the check failed: %v", err)
	}
	if !strings.Contains(line, want) {
		t.Fatalf("line %q does not say %q", line, want)
	}
}
