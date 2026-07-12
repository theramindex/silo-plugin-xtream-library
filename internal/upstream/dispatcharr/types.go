package dispatcharr

import (
	"bytes"
	"strconv"
)

type String string

func (s *String) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*s = ""
		return nil
	}
	if quoted, err := strconv.Unquote(string(data)); err == nil {
		*s = String(quoted)
		return nil
	}
	*s = String(string(data))
	return nil
}

func (s String) String() string {
	return string(s)
}

type Channel struct {
	ID                     String `json:"id"`
	UUID                   String `json:"uuid"`
	Name                   String `json:"name"`
	ChannelNumber          String `json:"channel_number"`
	TVGID                  String `json:"tvg_id"`
	EffectiveName          String `json:"effective_name"`
	EffectiveChannelNumber String `json:"effective_channel_number"`
	EffectiveTVGID         String `json:"effective_tvg_id"`
	EffectiveGroupID       String `json:"effective_channel_group_id"`
	EffectiveEPGDataID     String `json:"effective_epg_data_id"`
	LogoID                 String `json:"logo_id"`
	EffectiveLogoID        String `json:"effective_logo_id"`
	LogoURL                String `json:"logo_url"`
	HiddenFromOutput       bool   `json:"hidden_from_output"`
}

type ChannelGroup struct {
	ID   String `json:"id"`
	Name String `json:"name"`
}

type ChannelProfile struct {
	ID       String   `json:"id"`
	Name     String   `json:"name"`
	Channels []String `json:"channels"`
}

type CurrentUser struct {
	ID        String `json:"id"`
	Username  String `json:"username"`
	UserLevel int    `json:"user_level"`
}

type VersionInfo struct {
	Version   String `json:"version"`
	Timestamp String `json:"timestamp"`
}

type Program struct {
	ID          String `json:"id"`
	Title       String `json:"title"`
	SubTitle    String `json:"sub_title"`
	Description String `json:"description"`
	TVGID       String `json:"tvg_id"`
	StartTime   String `json:"start_time"`
	EndTime     String `json:"end_time"`
}

type ProgramSearchResult struct {
	Program
	Channels []ProgramChannel `json:"channels"`
}

type ProgramChannel struct {
	ID String `json:"id"`
}

type VODCategory struct {
	ID           String `json:"id"`
	Name         String `json:"name"`
	CategoryType String `json:"category_type"`
}

type Movie struct {
	ID          String `json:"id"`
	UUID        String `json:"uuid"`
	Name        String `json:"name"`
	CategoryID  String `json:"category"`
	Description String `json:"description"`
	Rating      String `json:"rating"`
	Year        String `json:"year"`
	Logo        Logo   `json:"logo"`
}

type Series struct {
	ID          String `json:"id"`
	UUID        String `json:"uuid"`
	Name        String `json:"name"`
	CategoryID  String `json:"category"`
	Description String `json:"description"`
	Rating      String `json:"rating"`
	Year        String `json:"year"`
	Logo        Logo   `json:"logo"`
}

type Logo struct {
	URL      String `json:"url"`
	CacheURL String `json:"cache_url"`
}
