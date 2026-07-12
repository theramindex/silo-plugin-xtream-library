package dispatcharr

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEndpointPreservesTrailingSlash(t *testing.T) {
	t.Parallel()

	client := NewLoginClient("https://dispatcharr.example.com", "demo", "secret")
	if got := client.endpoint("/api/accounts/token/"); got != "https://dispatcharr.example.com/api/accounts/token/" {
		t.Fatalf("unexpected endpoint: %q", got)
	}
}

func TestEndpointPreservesPaginationQuery(t *testing.T) {
	t.Parallel()

	client := NewLoginClient("https://dispatcharr.example.com", "demo", "secret")
	if got := client.endpoint("/api/channels/channels/?page=2"); got != "https://dispatcharr.example.com/api/channels/channels/?page=2" {
		t.Fatalf("unexpected pagination endpoint: %q", got)
	}
}

func TestCurrentUserReturnsRecordingPermissionLevel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/token/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access":"access-token","refresh":"refresh-token"}`))
		case "/api/accounts/users/me/":
			if r.Header.Get("Authorization") != "Bearer access-token" {
				http.Error(w, "missing auth", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":7,"username":"viewer","user_level":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewLoginClient(server.URL, "viewer", "secret")
	user, err := client.CurrentUser(t.Context())
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	if user.ID.String() != "7" || user.Username.String() != "viewer" || user.UserLevel != 1 {
		t.Fatalf("unexpected current user: %+v", user)
	}
}

func TestGetListRejectsCrossOriginPaginationWithoutForwardingCredentials(t *testing.T) {
	t.Parallel()

	attackerRequests := 0
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attackerRequests++
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("cross-origin request forwarded authorization header %q", got)
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer attacker.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"next":%q,"results":[]}`, attacker.URL+"/steal")
	}))
	defer upstream.Close()

	client := NewAPIKeyClient(upstream.URL, "top-secret")
	if _, err := client.ChannelGroups(t.Context()); err == nil {
		t.Fatal("expected cross-origin pagination link to be rejected")
	}
	if attackerRequests != 0 {
		t.Fatalf("expected no attacker requests, got %d", attackerRequests)
	}
}

func TestGetRawRelogsInWhenRefreshTokenExpired(t *testing.T) {
	t.Parallel()

	loginCount := 0
	refreshCount := 0
	resourceCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/token/":
			loginCount++
			w.Header().Set("Content-Type", "application/json")
			if loginCount == 1 {
				_, _ = w.Write([]byte(`{"access":"expired-access","refresh":"expired-refresh"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access":"fresh-access","refresh":"fresh-refresh"}`))
		case "/api/accounts/token/refresh/":
			refreshCount++
			http.Error(w, "refresh expired", http.StatusUnauthorized)
		case "/api/channels/":
			resourceCount++
			if r.Header.Get("Authorization") != "Bearer fresh-access" {
				http.Error(w, "stale access", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewLoginClient(server.URL, "demo", "secret")
	body, err := client.getRaw(t.Context(), "/api/channels/")
	if err != nil {
		t.Fatalf("get raw: %v", err)
	}
	if string(body) != `[]` {
		t.Fatalf("unexpected body: %s", body)
	}
	if loginCount != 2 {
		t.Fatalf("expected initial login and re-login, got %d", loginCount)
	}
	if refreshCount != 1 {
		t.Fatalf("expected one refresh attempt, got %d", refreshCount)
	}
	if resourceCount != 2 {
		t.Fatalf("expected original request and retry, got %d", resourceCount)
	}
}

func TestRefreshTokenStoresRotatedRefreshToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/token/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access":"expired-access","refresh":"old-refresh"}`))
		case "/api/accounts/token/refresh/":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access":"fresh-access","refresh":"new-refresh"}`))
		case "/api/channels/":
			if r.Header.Get("Authorization") != "Bearer fresh-access" {
				http.Error(w, "stale access", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewLoginClient(server.URL, "demo", "secret")
	if _, err := client.getRaw(t.Context(), "/api/channels/"); err != nil {
		t.Fatalf("get raw: %v", err)
	}

	client.mu.Lock()
	refresh := client.refresh
	client.mu.Unlock()
	if refresh != "new-refresh" {
		t.Fatalf("expected rotated refresh token, got %q", refresh)
	}
}
