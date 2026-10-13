package plugin

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
	"github.com/theramindex/silo-plugin-xtream-library/internal/config"
	"github.com/theramindex/silo-plugin-xtream-library/internal/model"
)

func liveTVFolderServer(t *testing.T, adminSettings string, source model.Source, channels []model.Channel) *HTTPRoutesServer {
	t.Helper()
	store := cache.NewStore()
	store.Replace(cache.Snapshot{Catalog: model.CatalogState{Source: source, Channels: channels}})
	store.SetAdminSettings(json.RawMessage(adminSettings))
	return NewHTTPRoutesServerWithSettings(store, func() config.Settings { return config.Settings{} })
}

func liveTVFolders(t *testing.T, server *HTTPRoutesServer) map[string][]string {
	t.Helper()
	response := liveTVContractGet(t, server, "/silo-compat/livetv/v1/channels", nil)
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.GetStatusCode(), response.GetBody())
	}
	var lineup []livetvChannel
	if err := json.Unmarshal(response.GetBody(), &lineup); err != nil {
		t.Fatalf("decode channels: %v", err)
	}
	out := map[string][]string{}
	for _, channel := range lineup {
		out[channel.ID] = channel.Folders
	}
	return out
}

// Xtream group names stay whole: the web app splits them only for a user who
// turns on groupCategoriesByPipe.
func TestLiveTVChannelFoldersKeepXtreamGroupsWhole(t *testing.T) {
	t.Parallel()

	server := liveTVFolderServer(t, `{"mode":"delimiter"}`, model.LiveTVSource(model.SourceModeXtream), []model.Channel{
		{ID: "flo", Name: "Flo Sports 1", CategoryID: "3", CategoryName: "USA | Flo Sports"},
	})

	if got, want := liveTVFolders(t, server)["flo"], []string{"USA | Flo Sports"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("folders = %v, want %v", got, want)
	}
}

// Other sources split group names into nested folders in delimiter mode.
func TestLiveTVChannelFoldersSplitM3UGroups(t *testing.T) {
	t.Parallel()

	server := liveTVFolderServer(t, `{"mode":"delimiter"}`, model.LiveTVSource(model.SourceModeM3UXMLTV), []model.Channel{
		{ID: "abc", Name: "WABC", CategoryID: "1", CategoryName: "US TV | Locals | ABC"},
	})

	if got, want := liveTVFolders(t, server)["abc"], []string{"US TV / Locals / ABC"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("folders = %v, want %v", got, want)
	}
}

// Without delimiter parsing a channel's folder is its whole group name.
func TestLiveTVChannelFoldersFallBackToGroupName(t *testing.T) {
	t.Parallel()

	server := liveTVFolderServer(t, `{"mode":"normal"}`, model.LiveTVSource(model.SourceModeM3UXMLTV), []model.Channel{
		{ID: "news", Name: "News HD", CategoryID: "7", CategoryName: "*USA | News"},
		{ID: "loose", Name: "No Group"},
	})

	got := liveTVFolders(t, server)
	if want := []string{"USA | News"}; !reflect.DeepEqual(got["news"], want) {
		t.Fatalf("news folders = %v, want %v", got["news"], want)
	}
	if got["loose"] != nil {
		t.Fatalf("a channel without a group has no folders, got %v", got["loose"])
	}
}

// Channel-name folders file "NY | WABC" under US TV / Locals / NY and other
// prefixes under an International root picked from the name and group.
func TestLiveTVChannelFoldersFromChannelNames(t *testing.T) {
	t.Parallel()

	server := liveTVFolderServer(t, `{"mode":"delimiter","virtualGroupSource":"channel"}`, model.LiveTVSource(model.SourceModeM3UXMLTV), []model.Channel{
		{ID: "wabc", Name: "NY | ABC | WABC", CategoryID: "1", CategoryName: "Locals"},
		{ID: "sky", Name: "UK | Sky Sports | Main Event", CategoryID: "2", CategoryName: "UK"},
		{ID: "bbc", Name: "UK | BBC One", CategoryID: "2", CategoryName: "UK"},
	})

	want := map[string][]string{
		"wabc": {"US TV / Locals / NY / ABC"},
		"sky":  {"International Sports / UK / Sky Sports"},
		"bbc":  {"International TV / UK / BBC One"},
	}
	if got := liveTVFolders(t, server); !reflect.DeepEqual(got, want) {
		t.Fatalf("folders = %v, want %v", got, want)
	}
}
