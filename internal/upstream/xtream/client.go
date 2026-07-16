package xtream

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	sharedhttp "github.com/theramindex/silo-plugin-xtream-library/internal/upstream/httpclient"
)

type Client struct {
	baseURL  string
	username string
	password string
	http     *http.Client
}

func NewClient(baseURL, username, password string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		http:     sharedhttp.New(),
	}
}

func (c *Client) TestConnection(ctx context.Context) error {
	var payload struct {
		UserInfo struct {
			Auth   json.RawMessage `json:"auth"`
			Status string          `json:"status"`
		} `json:"user_info"`
	}
	if err := c.getJSON(ctx, "", nil, &payload); err != nil {
		return fmt.Errorf("check account authentication: %w", err)
	}
	if !xtreamAuthAccepted(payload.UserInfo.Auth) {
		status := strings.TrimSpace(payload.UserInfo.Status)
		if status != "" {
			return fmt.Errorf("provider rejected credentials (account status %s)", status)
		}
		return fmt.Errorf("provider rejected credentials")
	}
	return nil
}

func xtreamAuthAccepted(value json.RawMessage) bool {
	normalized := strings.ToLower(strings.TrimSpace(string(value)))
	return normalized == "1" || normalized == `"1"` || normalized == "true" || normalized == `"true"`
}

func (c *Client) LiveCategories(ctx context.Context) ([]LiveCategory, error) {
	var categories []LiveCategory
	if err := c.getJSON(ctx, "get_live_categories", nil, &categories); err != nil {
		return nil, err
	}
	return categories, nil
}

func (c *Client) LiveStreams(ctx context.Context) ([]LiveStream, error) {
	var streams []LiveStream
	if err := c.getJSON(ctx, "get_live_streams", nil, &streams); err != nil {
		return nil, err
	}
	return streams, nil
}

func (c *Client) VODCategories(ctx context.Context) ([]VODCategory, error) {
	var categories []VODCategory
	if err := c.getJSON(ctx, "get_vod_categories", nil, &categories); err != nil {
		return nil, err
	}
	return categories, nil
}

func (c *Client) VODStreams(ctx context.Context) ([]VODStream, error) {
	var streams []VODStream
	if err := c.getJSON(ctx, "get_vod_streams", nil, &streams); err != nil {
		return nil, err
	}
	return streams, nil
}

func (c *Client) SeriesCategories(ctx context.Context) ([]SeriesCategory, error) {
	var categories []SeriesCategory
	if err := c.getJSON(ctx, "get_series_categories", nil, &categories); err != nil {
		return nil, err
	}
	return categories, nil
}

func (c *Client) Series(ctx context.Context) ([]Series, error) {
	var series []Series
	if err := c.getJSON(ctx, "get_series", nil, &series); err != nil {
		return nil, err
	}
	return series, nil
}

func (c *Client) SeriesInfo(ctx context.Context, seriesID int64) (SeriesInfo, error) {
	var response struct {
		Info     SeriesInfoMetadata       `json:"info"`
		Episodes map[string][]EpisodeInfo `json:"episodes"`
	}
	params := map[string]string{"series_id": strconv.FormatInt(seriesID, 10)}
	if err := c.getJSON(ctx, "get_series_info", params, &response); err != nil {
		return SeriesInfo{}, err
	}
	result := SeriesInfo{Info: response.Info}
	for season, episodes := range response.Episodes {
		seasonNumber, _ := strconv.Atoi(season)
		for index := range episodes {
			episodes[index].SeasonNumber = seasonNumber
		}
		result.Episodes = append(result.Episodes, episodes...)
	}
	sort.Slice(result.Episodes, func(left, right int) bool {
		if result.Episodes[left].SeasonNumber != result.Episodes[right].SeasonNumber {
			return result.Episodes[left].SeasonNumber < result.Episodes[right].SeasonNumber
		}
		return result.Episodes[left].EpisodeNumber < result.Episodes[right].EpisodeNumber
	})
	return result, nil
}

func (c *Client) ShortEPG(ctx context.Context, streamID int64) (ShortEPGResponse, error) {
	var response ShortEPGResponse
	params := map[string]string{"stream_id": strconv.FormatInt(streamID, 10)}
	if err := c.getJSON(ctx, "get_short_epg", params, &response); err != nil {
		return ShortEPGResponse{}, err
	}
	for index := range response.EPGListings {
		response.EPGListings[index].Title = DecodeEPGText(response.EPGListings[index].Title)
		response.EPGListings[index].Description = DecodeEPGText(response.EPGListings[index].Description)
	}
	return response, nil
}

func DecodeEPGText(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 8 {
		return value
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		decoded, err := encoding.DecodeString(trimmed)
		if err != nil || len(decoded) == 0 || !utf8.Valid(decoded) {
			continue
		}
		text := strings.TrimSpace(string(decoded))
		if text == "" {
			continue
		}
		printable := true
		for _, character := range text {
			if !unicode.IsPrint(character) && !unicode.IsSpace(character) {
				printable = false
				break
			}
		}
		if printable {
			return text
		}
	}
	return value
}

func (c *Client) ResolveLiveStreamURL(streamID int64) string {
	return c.ResolveLiveStreamURLWithExtension(streamID, "ts")
}

func (c *Client) ResolveLiveStreamURLWithExtension(streamID int64, extension string) string {
	resolved, err := url.Parse(c.baseURL)
	if err != nil {
		return ""
	}
	extension = strings.TrimPrefix(strings.TrimSpace(extension), ".")
	if extension != "m3u8" {
		extension = "ts"
	}
	resolved.Path = path.Join(resolved.Path, "live", c.username, c.password, strconv.FormatInt(streamID, 10)+"."+extension)
	return resolved.String()
}

func (c *Client) ResolveVODStreamURL(streamID int64, extension string) string {
	resolved, err := url.Parse(c.baseURL)
	if err != nil {
		return ""
	}
	extension = strings.TrimPrefix(strings.TrimSpace(extension), ".")
	if extension == "" {
		extension = "mp4"
	}
	resolved.Path = path.Join(resolved.Path, "movie", c.username, c.password, strconv.FormatInt(streamID, 10)+"."+extension)
	return resolved.String()
}

func (c *Client) ResolveEpisodeStreamURL(episode EpisodeInfo) string {
	return c.resolveMediaURL("series", episode.ID, episode.ContainerExtension)
}

func (c *Client) ResolveCatchupStreamURL(streamID int64, durationMinutes int, start string) string {
	if durationMinutes <= 0 || strings.TrimSpace(start) == "" {
		return ""
	}
	resolved, err := url.Parse(c.baseURL)
	if err != nil {
		return ""
	}
	resolved.Path = path.Join(resolved.Path, "timeshift", c.username, c.password, strconv.Itoa(durationMinutes), start, strconv.FormatInt(streamID, 10)+".ts")
	return resolved.String()
}

func (c *Client) resolveMediaURL(kind string, streamID int64, extension string) string {
	if streamID <= 0 {
		return ""
	}
	resolved, err := url.Parse(c.baseURL)
	if err != nil {
		return ""
	}
	extension = strings.TrimPrefix(strings.TrimSpace(extension), ".")
	if extension == "" {
		extension = "mp4"
	}
	resolved.Path = path.Join(resolved.Path, kind, c.username, c.password, strconv.FormatInt(streamID, 10)+"."+extension)
	return resolved.String()
}

func (c *Client) getJSON(ctx context.Context, action string, params map[string]string, target any) error {
	endpoint, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("parse base url: %w", err)
	}
	endpoint.Path = path.Join(endpoint.Path, "player_api.php")

	query := endpoint.Query()
	query.Set("username", c.username)
	query.Set("password", c.password)
	if action != "" {
		query.Set("action", action)
	}
	for key, value := range params {
		query.Set(key, value)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	response, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", sharedhttp.RedactErrorURL(err))
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("provider returned HTTP %d", response.StatusCode)
	}

	body, err := sharedhttp.ReadAllLimit(response.Body, sharedhttp.MaxJSONResponseBytes)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
