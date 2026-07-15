package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	DefaultChannelRefreshHours        = 24
	DefaultEPGRefreshHours            = 24
	MinimumDispatcharrVersion         = "0.27.1"
	AlternateEPGPolicyFillMissing     = "fill_missing"
	AlternateEPGPolicyPreferAlternate = "prefer_alternate"
	DefaultHLSBufferSeconds           = 12
	MinimumHLSBufferSeconds           = 5
	MaximumHLSBufferSeconds           = 60
)

type SourceMode string

const (
	SourceModeDirectLogin SourceMode = "direct_login"
	SourceModeAPIKey      SourceMode = "api_key"
	SourceModeXtream      SourceMode = "xtream"
	SourceModeM3UXMLTV    SourceMode = "m3u_xmltv"
)

type Settings struct {
	SourceMode        SourceMode
	DispatcharrURL    string
	DispatcharrUser   string
	DispatcharrPass   string
	DispatcharrAPIKey string
	ChannelProfile    string
	XtreamBaseURL     string
	XtreamUsername    string
	XtreamPassword    string
	XtreamLiveFormat  string
	XtreamSources     []XtreamSource
	M3UURL            string
	EPGXMLURL         string
	LiveTVEnabled     bool
	ChannelRefreshH   int
	EPGRefreshH       int
	ModeSwitchWarning string
	AdminSettings     json.RawMessage
}

type XtreamSource struct {
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	BaseURL             string          `json:"baseUrl"`
	Username            string          `json:"username"`
	Password            string          `json:"password"`
	LiveFormat          string          `json:"liveFormat"`
	HLSBufferSeconds    int             `json:"hlsBufferSeconds,omitempty"`
	Enabled             bool            `json:"enabled"`
	AlternateEPGEnabled bool            `json:"alternateEpgEnabled,omitempty"`
	AlternateEPGURL     string          `json:"alternateEpgUrl,omitempty"`
	AlternateEPGPolicy  string          `json:"alternateEpgPolicy,omitempty"`
	CatalogAccountID    string          `json:"catalogAccountId,omitempty"`
	Accounts            []XtreamAccount `json:"accounts,omitempty"`
}

type XtreamAccount struct {
	ID              string `json:"id"`
	Name            string `json:"name,omitempty"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	Enabled         bool   `json:"enabled"`
	Catalog         bool   `json:"catalog,omitempty"`
	Compatible      bool   `json:"compatible,omitempty"`
	ConnectionLimit int    `json:"connectionLimit,omitempty"`
}

func (s XtreamSource) EffectiveCatalogAccount() (XtreamAccount, bool) {
	for _, account := range s.Accounts {
		if account.ID == s.CatalogAccountID || account.Catalog {
			return account, true
		}
	}
	if strings.TrimSpace(s.Username) == "" || strings.TrimSpace(s.Password) == "" {
		return XtreamAccount{}, false
	}
	accountID := NormalizeSourceID(s.Username)
	if accountID == "" {
		accountID = "catalog"
	}
	return XtreamAccount{ID: accountID, Name: "Catalog", Username: s.Username, Password: s.Password, Enabled: true, Catalog: true, Compatible: true}, true
}

func (s XtreamSource) EffectivePlaybackAccounts() []XtreamAccount {
	if len(s.Accounts) == 0 {
		account, ok := s.EffectiveCatalogAccount()
		if !ok {
			return nil
		}
		return []XtreamAccount{account}
	}
	result := make([]XtreamAccount, 0, len(s.Accounts))
	for _, account := range s.Accounts {
		if account.Enabled && account.Compatible {
			result = append(result, account)
		}
	}
	return result
}

func (s XtreamSource) EffectiveLiveFormat() string {
	if strings.EqualFold(strings.TrimSpace(s.LiveFormat), "ts") {
		return "ts"
	}
	return "m3u8"
}

func (s XtreamSource) EffectiveHLSBufferSeconds() int {
	if s.HLSBufferSeconds <= 0 {
		return DefaultHLSBufferSeconds
	}
	if s.HLSBufferSeconds < MinimumHLSBufferSeconds {
		return MinimumHLSBufferSeconds
	}
	if s.HLSBufferSeconds > MaximumHLSBufferSeconds {
		return MaximumHLSBufferSeconds
	}
	return s.HLSBufferSeconds
}

func (s XtreamSource) EffectiveAlternateEPGPolicy() string {
	if strings.EqualFold(strings.TrimSpace(s.AlternateEPGPolicy), AlternateEPGPolicyPreferAlternate) {
		return AlternateEPGPolicyPreferAlternate
	}
	return AlternateEPGPolicyFillMissing
}

func (s Settings) EffectiveXtreamSources() []XtreamSource {
	if len(s.XtreamSources) == 0 {
		return []XtreamSource{{ID: "primary", Name: "Primary", BaseURL: s.XtreamBaseURL, Username: s.XtreamUsername, Password: s.XtreamPassword, LiveFormat: s.EffectiveXtreamLiveFormat(), Enabled: true}}
	}
	result := make([]XtreamSource, 0, len(s.XtreamSources))
	for _, source := range s.XtreamSources {
		if source.Enabled {
			result = append(result, source)
		}
	}
	return result
}

func (s Settings) XtreamSourceByID(id string) (XtreamSource, bool) {
	id = strings.TrimSpace(id)
	for _, source := range s.EffectiveXtreamSources() {
		if source.ID == id {
			return source, true
		}
	}
	return XtreamSource{}, false
}

func (s Settings) EffectiveXtreamLiveFormat() string {
	if strings.EqualFold(strings.TrimSpace(s.XtreamLiveFormat), "ts") {
		return "ts"
	}
	return "m3u8"
}

func (s Settings) EffectiveSourceMode() SourceMode {
	if s.SourceMode != "" {
		return s.SourceMode
	}
	if strings.TrimSpace(s.DispatcharrAPIKey) != "" {
		return SourceModeAPIKey
	}
	if strings.TrimSpace(s.DispatcharrURL) != "" {
		return SourceModeDirectLogin
	}
	return ""
}

func (s *Settings) Validate() error {
	s.SourceMode = s.EffectiveSourceMode()

	switch s.SourceMode {
	case SourceModeDirectLogin:
		if strings.TrimSpace(s.DispatcharrURL) == "" {
			return fmt.Errorf("dispatcharr url is required")
		}
		if strings.TrimSpace(s.DispatcharrUser) == "" {
			return fmt.Errorf("dispatcharr username is required")
		}
		if strings.TrimSpace(s.DispatcharrPass) == "" {
			return fmt.Errorf("dispatcharr password is required")
		}
	case SourceModeAPIKey:
		if strings.TrimSpace(s.DispatcharrURL) == "" {
			return fmt.Errorf("dispatcharr url is required")
		}
		if strings.TrimSpace(s.DispatcharrAPIKey) == "" {
			return fmt.Errorf("dispatcharr api key is required")
		}
	case SourceModeXtream:
		sources := s.EffectiveXtreamSources()
		if len(sources) == 0 {
			return fmt.Errorf("at least one enabled xtream source is required")
		}
		seen := map[string]bool{}
		for _, source := range sources {
			if strings.TrimSpace(source.ID) == "" {
				return fmt.Errorf("xtream source id is required")
			}
			if seen[source.ID] {
				return fmt.Errorf("xtream source id %q is duplicated", source.ID)
			}
			seen[source.ID] = true
			if strings.TrimSpace(source.BaseURL) == "" || strings.TrimSpace(source.Username) == "" || strings.TrimSpace(source.Password) == "" {
				return fmt.Errorf("xtream source %q credentials are incomplete", source.ID)
			}
		}
	case SourceModeM3UXMLTV:
		if strings.TrimSpace(s.M3UURL) == "" {
			return fmt.Errorf("m3u url is required")
		}
		if strings.TrimSpace(s.EPGXMLURL) == "" {
			return fmt.Errorf("epg xml url is required")
		}
	default:
		return fmt.Errorf("source mode is required")
	}

	if s.ChannelRefreshH == 0 {
		s.ChannelRefreshH = DefaultChannelRefreshHours
	}
	if s.EPGRefreshH == 0 {
		s.EPGRefreshH = DefaultEPGRefreshHours
	}
	if s.ChannelRefreshH <= 0 {
		return fmt.Errorf("channel refresh interval must be positive")
	}
	if s.EPGRefreshH <= 0 {
		return fmt.Errorf("epg refresh interval must be positive")
	}

	return nil
}

func CatalogCacheKey(settings Settings) string {
	parts := []string{
		string(settings.EffectiveSourceMode()),
		strings.TrimSpace(settings.DispatcharrURL),
		strings.TrimSpace(settings.DispatcharrUser),
		strings.TrimSpace(settings.DispatcharrAPIKey),
		strings.TrimSpace(settings.ChannelProfile),
		strings.TrimSpace(settings.XtreamBaseURL),
		strings.TrimSpace(settings.XtreamUsername),
		strings.TrimSpace(settings.M3UURL),
		strings.TrimSpace(settings.EPGXMLURL),
	}
	for _, source := range settings.EffectiveXtreamSources() {
		parts = append(parts, source.ID, source.Name, source.BaseURL, source.Username, source.EffectiveLiveFormat(), source.AlternateEPGURL, source.EffectiveAlternateEPGPolicy(), fmt.Sprintf("%t", source.AlternateEPGEnabled))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}
