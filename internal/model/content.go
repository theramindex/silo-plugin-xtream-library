package model

type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type VODItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	CategoryID  string `json:"categoryId,omitempty"`
	PosterURL   string `json:"posterUrl,omitempty"`
	Rating      string `json:"rating,omitempty"`
	Added       string `json:"added,omitempty"`
	Container   string `json:"container,omitempty"`
	StreamURL   string `json:"streamUrl,omitempty"`
	Description string `json:"description,omitempty"`
}

type SeriesItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	CategoryID  string `json:"categoryId,omitempty"`
	PosterURL   string `json:"posterUrl,omitempty"`
	Rating      string `json:"rating,omitempty"`
	ReleaseDate string `json:"releaseDate,omitempty"`
	Description string `json:"description,omitempty"`
}

type ContentState struct {
	LiveCategories   []Category   `json:"liveCategories"`
	VODCategories    []Category   `json:"vodCategories"`
	VODItems         []VODItem    `json:"vodItems"`
	SeriesCategories []Category   `json:"seriesCategories"`
	SeriesItems      []SeriesItem `json:"seriesItems"`
}
