package main

import (
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

func TestRewritePublicManifestMatchesRuntimeAdminRoutes(t *testing.T) {
	manifest := &pluginv1.PluginManifest{HttpRoutes: []*pluginv1.HttpRouteDescriptor{
		{Id: "dispatcharr-admin", Path: "/dispatcharr/admin"},
		{Id: "dispatcharr-api-admin-sources", Path: "/dispatcharr/api/admin-sources"},
		{Id: "dispatcharr-recordings", Path: "/dispatcharr/recordings"},
	}}

	rewritePublicManifestForXtream(manifest)

	if len(manifest.GetHttpRoutes()) != 2 {
		t.Fatalf("expected admin routes to remain while retired DVR routes are removed, got %+v", manifest.GetHttpRoutes())
	}
	if got := manifest.GetHttpRoutes()[0]; got.GetId() != "xtream-admin" || got.GetPath() != "/xtream/admin" {
		t.Fatalf("unexpected rewritten admin route: %+v", got)
	}
	if got := manifest.GetHttpRoutes()[1]; got.GetId() != "xtream-api-admin-sources" || got.GetPath() != "/xtream/api/admin-sources" {
		t.Fatalf("unexpected rewritten admin API route: %+v", got)
	}
}
