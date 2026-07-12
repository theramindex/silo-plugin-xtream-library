package plugin

import (
	"encoding/json"
	"testing"

	"github.com/theramindex/silo-plugin-dispatcharr/internal/model"
)

func TestProviderDescriptorUsesLiveTVIdentity(t *testing.T) {
	t.Parallel()

	descriptor := ProviderDescriptor()
	if descriptor.SourceID != model.LiveTVSourceID {
		t.Fatalf("expected live tv source id, got %q", descriptor.SourceID)
	}
	if descriptor.Name != "Live TV" {
		t.Fatalf("expected live tv name, got %q", descriptor.Name)
	}
}

func TestHealthPayloadJSONIncludesStatusFields(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(HealthPayload{Status: "ok", SourceName: "Live TV", LastSuccessUnix: 100})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected payload bytes")
	}
}
