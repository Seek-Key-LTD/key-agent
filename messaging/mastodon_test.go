// Copyright 2026 Google LLC
//
// Unit tests for Mastodon ActivityPub Channel Adapter

package messaging_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/adk/v2/messaging"
)

func TestMastodonClient_Mock(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/accounts/verify_credentials":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id":"1001","username":"test_agent","acct":"test_agent","display_name":"Test Agent","url":"https://test/@test"}`))
		case "/api/v1/statuses":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id":"2002","created_at":"2026-09-16T10:00:00Z","content":"hello world","account":{"id":"1001","username":"test_agent"}}`))
		case "/api/v1/notifications":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"id":"3003","type":"mention","created_at":"2026-09-16T10:00:00Z","account":{"id":"1002","username":"director"},"status":{"id":"4004","content":"action!"}}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	ctx := context.Background()
	client := messaging.NewMastodonClient(messaging.MastodonConfig{
		ServerURL:   ts.URL,
		AccessToken: "mock_token_abc",
		AgentName:   "test_agent",
	})

	// 1. Test VerifyCredentials
	acc, err := client.VerifyCredentials(ctx)
	if err != nil {
		t.Fatalf("VerifyCredentials failed: %v", err)
	}
	if acc.Username != "test_agent" {
		t.Errorf("expected username test_agent, got %s", acc.Username)
	}

	// 2. Test PostStatus
	st, err := client.PostStatus(ctx, "hello world", "", "public")
	if err != nil {
		t.Fatalf("PostStatus failed: %v", err)
	}
	if st.ID != "2002" {
		t.Errorf("expected status ID 2002, got %s", st.ID)
	}

	// 3. Test GetNotifications
	notes, err := client.GetNotifications(ctx, "", []string{"mention"})
	if err != nil {
		t.Fatalf("GetNotifications failed: %v", err)
	}
	if len(notes) != 1 || notes[0].Type != "mention" {
		t.Errorf("unexpected notifications: %v", notes)
	}
}
