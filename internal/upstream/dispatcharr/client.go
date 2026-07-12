package dispatcharr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	sharedhttp "github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/httpclient"
)

const maxPaginationPages = 2000

var errPaginationLimit = fmt.Errorf("pagination exceeded %d pages", maxPaginationPages)

type Client struct {
	baseURL  string
	username string
	password string
	apiKey   string
	http     *http.Client

	mu      sync.Mutex
	access  string
	refresh string
}

func NewLoginClient(baseURL, username, password string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), username: username, password: password, http: sharedhttp.New()}
}

func NewAPIKeyClient(baseURL, apiKey string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, http: sharedhttp.New()}
}

func (c *Client) TestConnection(ctx context.Context) error {
	var target map[string]any
	return c.getJSON(ctx, "/api/accounts/users/me/", &target)
}

func (c *Client) Version(ctx context.Context) (VersionInfo, error) {
	var version VersionInfo
	return version, c.getJSON(ctx, "/api/core/version/", &version)
}

func (c *Client) Channels(ctx context.Context) ([]Channel, error) {
	var channels []Channel
	return channels, c.getList(ctx, "/api/channels/channels/", &channels)
}

func (c *Client) ChannelGroups(ctx context.Context) ([]ChannelGroup, error) {
	var groups []ChannelGroup
	return groups, c.getList(ctx, "/api/channels/groups/", &groups)
}

func (c *Client) ChannelProfiles(ctx context.Context) ([]ChannelProfile, error) {
	var profiles []ChannelProfile
	return profiles, c.getList(ctx, "/api/channels/profiles/", &profiles)
}

func (c *Client) CurrentUser(ctx context.Context) (CurrentUser, error) {
	var user CurrentUser
	return user, c.getJSON(ctx, "/api/accounts/users/me/", &user)
}

func (c *Client) Programs(ctx context.Context) ([]Program, error) {
	var response struct {
		Data []Program `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/epg/grid/", &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

func (c *Client) SearchPrograms(ctx context.Context, start, end time.Time) ([]ProgramSearchResult, error) {
	var programs []ProgramSearchResult
	next := c.searchProgramsEndpoint(start, end, 1)
	seen := map[string]bool{}
	pages := 0
	for strings.TrimSpace(next) != "" {
		if seen[next] {
			return nil, fmt.Errorf("pagination cycle detected")
		}
		seen[next] = true
		pages++
		if pages > maxPaginationPages {
			return nil, errPaginationLimit
		}
		var page struct {
			Next    string                `json:"next"`
			Results []ProgramSearchResult `json:"results"`
		}
		if err := c.getJSON(ctx, next, &page); err != nil {
			return nil, err
		}
		programs = append(programs, page.Results...)
		next = page.Next
	}
	return programs, nil
}

func (c *Client) searchProgramsEndpoint(start, end time.Time, page int) string {
	values := url.Values{}
	values.Set("start_after", start.UTC().Format(time.RFC3339))
	values.Set("start_before", end.UTC().Format(time.RFC3339))
	values.Set("page_size", "500")
	values.Set("page", fmt.Sprintf("%d", page))
	return "/api/epg/programs/search/?" + values.Encode()
}

func (c *Client) VODCategories(ctx context.Context) ([]VODCategory, error) {
	var categories []VODCategory
	return categories, c.getList(ctx, "/api/vod/categories/", &categories)
}

func (c *Client) Movies(ctx context.Context) ([]Movie, error) {
	var movies []Movie
	return movies, c.getList(ctx, "/api/vod/movies/", &movies)
}

func (c *Client) Series(ctx context.Context) ([]Series, error) {
	var series []Series
	return series, c.getList(ctx, "/api/vod/series/", &series)
}

func (c *Client) Recordings(ctx context.Context) ([]json.RawMessage, error) {
	var recordings []json.RawMessage
	return recordings, c.getList(ctx, "/api/channels/recordings/", &recordings)
}

func (c *Client) CreateRecording(ctx context.Context, payload any) (json.RawMessage, error) {
	return c.postJSON(ctx, "/api/channels/recordings/", payload)
}

func (c *Client) LiveStreamURL(channelUUID string) string {
	return c.absolutePath(path.Join("/proxy/ts/stream", strings.TrimSpace(channelUUID)))
}

func (c *Client) LogoCacheURL(logoID string) string {
	logoID = strings.TrimSpace(logoID)
	if logoID == "" {
		return ""
	}
	return c.absolutePath("/api/channels/logos/" + logoID + "/cache/")
}

func (c *Client) MovieStreamURL(movieUUID string) string {
	return c.absolutePath(path.Join("/proxy/vod/movie", strings.TrimSpace(movieUUID)))
}

func (c *Client) SeriesStreamURL(seriesUUID string) string {
	return c.absolutePath(path.Join("/proxy/vod/series", strings.TrimSpace(seriesUUID)))
}

func (c *Client) AbsoluteURL(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err == nil && parsed.IsAbs() {
		return raw
	}
	return c.absolutePath(raw)
}

func (c *Client) getList(ctx context.Context, endpoint string, target any) error {
	next := endpoint
	seen := map[string]bool{}
	pages := 0
	for strings.TrimSpace(next) != "" {
		if seen[next] {
			return fmt.Errorf("pagination cycle detected")
		}
		seen[next] = true
		pages++
		if pages > maxPaginationPages {
			return errPaginationLimit
		}
		var page struct {
			Next    string          `json:"next"`
			Results json.RawMessage `json:"results"`
		}
		raw, err := c.getRaw(ctx, next)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &page); err == nil && len(page.Results) > 0 {
			if err := appendJSONList(page.Results, target); err != nil {
				return err
			}
			next = page.Next
			continue
		}
		return appendJSONList(raw, target)
	}
	return nil
}

func appendJSONList(raw []byte, target any) error {
	var current []json.RawMessage
	if err := json.Unmarshal(raw, &current); err != nil {
		return fmt.Errorf("decode list: %w", err)
	}

	out, err := json.Marshal(current)
	if err != nil {
		return err
	}

	switch values := target.(type) {
	case *[]Channel:
		var next []Channel
		if err := json.Unmarshal(out, &next); err != nil {
			return err
		}
		*values = append(*values, next...)
	case *[]ChannelGroup:
		var next []ChannelGroup
		if err := json.Unmarshal(out, &next); err != nil {
			return err
		}
		*values = append(*values, next...)
	case *[]ChannelProfile:
		var next []ChannelProfile
		if err := json.Unmarshal(out, &next); err != nil {
			return err
		}
		*values = append(*values, next...)
	case *[]VODCategory:
		var next []VODCategory
		if err := json.Unmarshal(out, &next); err != nil {
			return err
		}
		*values = append(*values, next...)
	case *[]Movie:
		var next []Movie
		if err := json.Unmarshal(out, &next); err != nil {
			return err
		}
		*values = append(*values, next...)
	case *[]Series:
		var next []Series
		if err := json.Unmarshal(out, &next); err != nil {
			return err
		}
		*values = append(*values, next...)
	case *[]json.RawMessage:
		*values = append(*values, current...)
	default:
		return fmt.Errorf("unsupported list target %T", target)
	}
	return nil
}

func (c *Client) getJSON(ctx context.Context, endpoint string, target any) error {
	raw, err := c.getRaw(ctx, endpoint)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) getRaw(ctx context.Context, endpoint string) ([]byte, error) {
	return c.getRawWithRetry(ctx, endpoint, true)
}

func (c *Client) postJSON(ctx context.Context, endpoint string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return c.postRawWithRetry(ctx, endpoint, body, true)
}

func (c *Client) postRawWithRetry(ctx context.Context, endpoint string, body []byte, allowRefresh bool) ([]byte, error) {
	if err := c.ensureAuth(ctx); err != nil {
		return nil, err
	}
	target, err := c.requestEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Silo Dispatcharr Plugin")
	c.authorize(req)
	response, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", sharedhttp.RedactErrorURL(err))
	}
	defer response.Body.Close()
	if allowRefresh && response.StatusCode == http.StatusUnauthorized && c.canRecoverAuth() {
		if err := c.recoverAuth(ctx); err == nil {
			return c.postRawWithRetry(ctx, endpoint, body, false)
		}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d: %s", response.StatusCode, responseSnippet(response.Body))
	}
	return sharedhttp.ReadAllLimit(response.Body, sharedhttp.MaxJSONResponseBytes)
}

func (c *Client) getRawWithRetry(ctx context.Context, endpoint string, allowRefresh bool) ([]byte, error) {
	if err := c.ensureAuth(ctx); err != nil {
		return nil, err
	}
	target, err := c.requestEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	c.authorize(req)
	response, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", sharedhttp.RedactErrorURL(err))
	}
	defer response.Body.Close()
	if allowRefresh && response.StatusCode == http.StatusUnauthorized && c.canRecoverAuth() {
		if err := c.recoverAuth(ctx); err == nil {
			return c.getRawWithRetry(ctx, endpoint, false)
		}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	return sharedhttp.ReadAllLimit(response.Body, sharedhttp.MaxJSONResponseBytes)
}

func (c *Client) ensureAuth(ctx context.Context) error {
	if strings.TrimSpace(c.apiKey) != "" {
		return nil
	}
	c.mu.Lock()
	hasAccess := c.access != ""
	c.mu.Unlock()
	if hasAccess {
		return nil
	}
	return c.login(ctx)
}

func (c *Client) login(ctx context.Context) error {
	payload, err := json.Marshal(map[string]string{"username": c.username, "password": c.password})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/api/accounts/token/"), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Silo Dispatcharr Plugin")
	response, err := c.http.Do(req)
	if err != nil {
		return sharedhttp.RedactErrorURL(err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("dispatcharr login status %d: %s", response.StatusCode, responseSnippet(response.Body))
	}
	var token struct {
		Access  string `json:"access"`
		Refresh string `json:"refresh"`
	}
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		return err
	}
	c.mu.Lock()
	c.access = token.Access
	c.refresh = token.Refresh
	c.mu.Unlock()
	return nil
}

func (c *Client) refreshToken(ctx context.Context) error {
	c.mu.Lock()
	refresh := c.refresh
	c.mu.Unlock()
	if refresh == "" {
		return fmt.Errorf("missing refresh token")
	}
	payload, err := json.Marshal(map[string]string{"refresh": refresh})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/api/accounts/token/refresh/"), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Silo Dispatcharr Plugin")
	response, err := c.http.Do(req)
	if err != nil {
		return sharedhttp.RedactErrorURL(err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("dispatcharr refresh status %d: %s", response.StatusCode, responseSnippet(response.Body))
	}
	var token struct {
		Access  string `json:"access"`
		Refresh string `json:"refresh"`
	}
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		return err
	}
	c.mu.Lock()
	c.access = token.Access
	if token.Refresh != "" {
		c.refresh = token.Refresh
	}
	c.mu.Unlock()
	return nil
}

func (c *Client) canRecoverAuth() bool {
	if strings.TrimSpace(c.apiKey) != "" {
		return false
	}
	c.mu.Lock()
	refresh := c.refresh
	c.mu.Unlock()
	return refresh != "" || (strings.TrimSpace(c.username) != "" && strings.TrimSpace(c.password) != "")
}

func (c *Client) recoverAuth(ctx context.Context) error {
	if c.refreshToken(ctx) == nil {
		return nil
	}
	c.mu.Lock()
	c.access = ""
	c.refresh = ""
	c.mu.Unlock()
	return c.login(ctx)
}

func responseSnippet(reader io.Reader) string {
	body, err := io.ReadAll(io.LimitReader(reader, 240))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}

func (c *Client) authorize(req *http.Request) {
	if strings.TrimSpace(c.apiKey) != "" {
		req.Header.Set("X-API-Key", c.apiKey)
		req.Header.Set("Authorization", "ApiKey "+c.apiKey)
		return
	}
	c.mu.Lock()
	access := c.access
	c.mu.Unlock()
	if access != "" {
		req.Header.Set("Authorization", "Bearer "+access)
	}
}

func (c *Client) endpoint(endpoint string) string {
	target, err := c.requestEndpoint(endpoint)
	if err != nil {
		return ""
	}
	return target
}

func (c *Client) requestEndpoint(endpoint string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err == nil && parsed.IsAbs() {
		base, baseErr := url.Parse(c.baseURL)
		if baseErr != nil {
			return "", fmt.Errorf("parse dispatcharr base url: %w", baseErr)
		}
		if !strings.EqualFold(parsed.Scheme, base.Scheme) || !strings.EqualFold(parsed.Host, base.Host) {
			return "", fmt.Errorf("refusing cross-origin dispatcharr endpoint")
		}
		return parsed.String(), nil
	}
	if err != nil {
		return "", fmt.Errorf("parse dispatcharr endpoint: %w", err)
	}
	target := c.absolutePath(endpoint)
	if target == "" {
		return "", fmt.Errorf("resolve dispatcharr endpoint")
	}
	return target, nil
}

func (c *Client) absolutePath(rawPath string) string {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return ""
	}
	relative, err := url.Parse(rawPath)
	if err != nil {
		return ""
	}
	if relative.IsAbs() {
		return rawPath
	}
	base.Path = path.Join(base.Path, relative.Path)
	if strings.HasSuffix(relative.Path, "/") && !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	base.RawQuery = relative.RawQuery
	base.Fragment = ""
	return base.String()
}
