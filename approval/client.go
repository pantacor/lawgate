package approval

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	requestTimeout = 30 * time.Second
	// maxPages bounds a listing, so a server that always names a next page
	// cannot keep the gate running.
	maxPages = 100
	pageSize = "100"
	// maxBody bounds the JSON read from one response.
	maxBody = 8 << 20
	// maxErrorBody bounds the part of an error response read for its message.
	maxErrorBody = 4 << 10
)

// pagination is how a platform names the next page of a listing.
type pagination int

const (
	// linkHeader is GitHub's Link: <url>; rel="next".
	linkHeader pagination = iota
	// nextPageHeader is GitLab's X-Next-Page: <number>.
	nextPageHeader
)

// client reads one platform's REST API with a token. The token travels only
// in a request header, and never leaves the API's host.
type client struct {
	base      *url.URL
	header    http.Header
	token     string
	tokenName string
	pages     pagination
	http      *http.Client
}

func newClient(base, tokenName, token string, header http.Header, pages pagination) (client, error) {
	parsed, err := url.Parse(strings.TrimSuffix(base, "/") + "/")
	webScheme := err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http")
	if !webScheme || parsed.Host == "" {
		return client{}, fmt.Errorf("API URL %q is not an absolute http(s) URL", base)
	}
	api := client{base: parsed, header: header, token: token, tokenName: tokenName, pages: pages}
	api.http = &http.Client{Timeout: requestTimeout, CheckRedirect: api.sameHost}
	return api, nil
}

// sameHost refuses a redirect to another host, which would receive the
// token header.
func (api client) sameHost(request *http.Request, _ []*http.Request) error {
	if api.onHost(request.URL) {
		return nil
	}
	return fmt.Errorf("redirect to %s refused: it leaves the API host", request.URL.Host)
}

func (api client) onHost(target *url.URL) bool {
	return target.Scheme == api.base.Scheme && target.Host == api.base.Host
}

// endpoint is path under the API base, with query.
func (api client) endpoint(path string, query url.Values) (*url.URL, error) {
	reference, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("API path %q: %w", path, err)
	}
	target := api.base.ResolveReference(reference)
	target.RawQuery = query.Encode()
	return target, nil
}

// getOne reads one JSON object from path.
func (api client) getOne(path string, into any) error {
	target, err := api.endpoint(path, nil)
	if err != nil {
		return err
	}
	_, err = api.get(target, into)
	return err
}

// get reads target into into and returns the next page, nil on the last.
func (api client) get(target *url.URL, into any) (*url.URL, error) {
	request, err := http.NewRequest(http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", target.Path, err)
	}
	request.Header = api.header.Clone()
	response, err := api.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %s", target.Path, api.redact(transportError(err)))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, api.statusError(target, response)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxBody))
	if err := decoder.Decode(into); err != nil {
		return nil, fmt.Errorf("GET %s: unexpected answer: %w", target.Path, err)
	}
	return api.nextPage(response, target)
}

// transportError drops the URL net/http wraps around a failure: the
// message names the path once, without the query.
func transportError(err error) string {
	var wrapped *url.Error
	if errors.As(err, &wrapped) {
		return wrapped.Err.Error()
	}
	return err.Error()
}

// statusError says what the platform refused, and which token to check.
func (api client) statusError(target *url.URL, response *http.Response) error {
	text := fmt.Sprintf("GET %s: %s", target.Path, response.Status)
	if message := errorMessage(response.Body); message != "" {
		text += " (" + message + ")"
	}
	denied := response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden
	if denied || response.StatusCode == http.StatusNotFound {
		text += fmt.Sprintf(": check that %s can read this project's merge requests or pull requests", api.tokenName)
	}
	return errors.New(api.redact(text))
}

// errorMessage is the "message" string of a JSON error body, if any.
func errorMessage(body io.Reader) string {
	var answer struct {
		Message json.RawMessage `json:"message"`
	}
	data, _ := io.ReadAll(io.LimitReader(body, maxErrorBody))
	if json.Unmarshal(data, &answer) != nil {
		return ""
	}
	var message string
	if json.Unmarshal(answer.Message, &message) != nil {
		return ""
	}
	return truncate(message, 200)
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

// redact removes the token from a message, whatever a server echoed.
func (api client) redact(text string) string {
	if api.token == "" {
		return text
	}
	return strings.ReplaceAll(text, api.token, "[token]")
}

func (api client) nextPage(response *http.Response, current *url.URL) (*url.URL, error) {
	if api.pages == nextPageHeader {
		return gitlabNextPage(response, current)
	}
	return api.githubNextPage(response, current)
}

// githubNextPage follows the Link header's rel="next", on the API's host only.
func (api client) githubNextPage(response *http.Response, current *url.URL) (*url.URL, error) {
	link := nextLink(response.Header.Get("Link"))
	if link == "" {
		return nil, nil
	}
	next, err := current.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("GET %s: next page link: %w", current.Path, err)
	}
	if !api.onHost(next) {
		return nil, fmt.Errorf("GET %s: next page leaves the API host (%s)", current.Path, next.Host)
	}
	return next, nil
}

// nextLink is the target of rel="next" in a Link header.
func nextLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		target, parameters, _ := strings.Cut(part, ";")
		if hasNextRelation(parameters) {
			return strings.Trim(strings.TrimSpace(target), "<>")
		}
	}
	return ""
}

func hasNextRelation(parameters string) bool {
	for _, parameter := range strings.Split(parameters, ";") {
		if strings.TrimSpace(parameter) == `rel="next"` {
			return true
		}
	}
	return false
}

// gitlabNextPage sets the page X-Next-Page names on the current URL.
func gitlabNextPage(response *http.Response, current *url.URL) (*url.URL, error) {
	page := strings.TrimSpace(response.Header.Get("X-Next-Page"))
	if page == "" {
		return nil, nil
	}
	number, err := strconv.Atoi(page)
	if err != nil || number < 1 {
		return nil, fmt.Errorf("GET %s: invalid X-Next-Page %q", current.Path, page)
	}
	next := *current
	query := next.Query()
	query.Set("page", page)
	next.RawQuery = query.Encode()
	return &next, nil
}

// eachPage reads every page of a listing at path and hands each to visit,
// until visit returns false or the last page.
func eachPage[T any](api client, path string, query url.Values, visit func([]T) bool) error {
	query = cloneQuery(query)
	query.Set("per_page", pageSize)
	target, err := api.endpoint(path, query)
	if err != nil {
		return err
	}
	for page := 0; target != nil; page++ {
		if page == maxPages {
			return fmt.Errorf("GET %s: more than %d pages", path, maxPages)
		}
		var items []T
		next, err := api.get(target, &items)
		if err != nil || !visit(items) {
			return err
		}
		target = next
	}
	return nil
}

// getPages reads every item of a listing at path.
func getPages[T any](api client, path string, query url.Values) ([]T, error) {
	var all []T
	err := eachPage(api, path, query, func(items []T) bool {
		all = append(all, items...)
		return true
	})
	return all, err
}

func cloneQuery(query url.Values) url.Values {
	cloned := url.Values{}
	for key, values := range query {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}
