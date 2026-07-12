package xtream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	sharedhttp "github.com/theramindex/silo-plugin-dispatcharr/internal/upstream/httpclient"
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
	_, err := c.LiveCategories(ctx)
	return err
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

func (c *Client) ShortEPG(ctx context.Context, streamID int64) (ShortEPGResponse, error) {
	var response ShortEPGResponse
	params := map[string]string{"stream_id": strconv.FormatInt(streamID, 10)}
	if err := c.getJSON(ctx, "get_short_epg", params, &response); err != nil {
		return ShortEPGResponse{}, err
	}
	return response, nil
}

func (c *Client) ResolveLiveStreamURL(streamID int64) string {
	resolved, err := url.Parse(c.baseURL)
	if err != nil {
		return ""
	}
	resolved.Path = path.Join(resolved.Path, "live", c.username, c.password, strconv.FormatInt(streamID, 10)+".ts")
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

func (c *Client) getJSON(ctx context.Context, action string, params map[string]string, target any) error {
	endpoint, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("parse base url: %w", err)
	}
	endpoint.Path = path.Join(endpoint.Path, "player_api.php")

	query := endpoint.Query()
	query.Set("username", c.username)
	query.Set("password", c.password)
	query.Set("action", action)
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
		return fmt.Errorf("unexpected status %d", response.StatusCode)
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
