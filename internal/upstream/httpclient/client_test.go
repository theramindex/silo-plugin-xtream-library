package httpclient

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNewAllowsSlowRemoteCatalogResponses(t *testing.T) {
	t.Parallel()

	if timeout := New().Timeout; timeout != 30*time.Second {
		t.Fatalf("HTTP client timeout = %s, want 30s", timeout)
	}
}

func TestRedactErrorURLPreservesCauseWithoutCredentials(t *testing.T) {
	t.Parallel()

	cause := errors.New("dial failed")
	err := RedactErrorURL(&url.Error{Op: "Get", URL: "https://provider.example/player?username=demo&password=secret", Err: cause})
	if !errors.Is(err, cause) {
		t.Fatalf("expected wrapped transport cause, got %v", err)
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "provider.example") {
		t.Fatalf("transport error exposed request URL: %v", err)
	}
}

func TestReadAllLimitRejectsOversizedPayload(t *testing.T) {
	t.Parallel()

	if _, err := ReadAllLimit(strings.NewReader("12345"), 4); err == nil {
		t.Fatal("expected oversized response to be rejected")
	}
}

func TestReadAllLimitReturnsPayloadWithinLimit(t *testing.T) {
	t.Parallel()

	data, err := ReadAllLimit(strings.NewReader("1234"), 4)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if string(data) != "1234" {
		t.Fatalf("unexpected payload %q", data)
	}
}
