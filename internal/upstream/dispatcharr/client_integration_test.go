package dispatcharr

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLoginClientIntegration(t *testing.T) {
	baseURL := os.Getenv("DISPATCHARR_TEST_URL")
	username := os.Getenv("DISPATCHARR_TEST_USERNAME")
	password := os.Getenv("DISPATCHARR_TEST_PASSWORD")
	if baseURL == "" || username == "" || password == "" {
		t.Skip("set DISPATCHARR_TEST_URL, DISPATCHARR_TEST_USERNAME, and DISPATCHARR_TEST_PASSWORD")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client := NewLoginClient(baseURL, username, password)
	if err := client.TestConnection(ctx); err != nil {
		t.Fatalf("test connection: %v", err)
	}
}
