// Copyright 2026 Google LLC
//
// Unit tests for Matrix Unified Communications & Reaction

package messaging_test

import (
	"context"
	"testing"

	"google.golang.org/adk/v2/messaging"
)

func TestMatrixClient_SendMessageAndReaction(t *testing.T) {
	ctx := context.Background()

	client := messaging.NewMatrixClient(messaging.MatrixConfig{
		HomeserverURL: "https://matrix.capitaltrain.cn",
		AccessToken:   "syt_test_token_123",
		DefaultRoomID: "!room:capitaltrain.cn",
	})

	// 1. Test Sending Message (BP-Pager Webhook)
	evtID, err := client.SendMessage(ctx, "!room:capitaltrain.cn", "Pager Alert: New Task assigned in Gitea")
	if err != nil {
		t.Fatalf("unexpected error sending message: %v", err)
	}
	if evtID == "" {
		t.Errorf("expected non-empty event_id")
	}

	// 2. Test Adding Reaction (m.reaction / m.annotation)
	reactID, err := client.SendReaction(ctx, "!room:capitaltrain.cn", evtID, "👀")
	if err != nil {
		t.Fatalf("unexpected error adding reaction: %v", err)
	}
	if reactID == "" {
		t.Errorf("expected non-empty reaction_id")
	}

	// 3. Test Email Fallback
	if err := client.SendEmailNotice(ctx, "dev@capitaltrain.cn", "Task Notice", "Task Assigned"); err != nil {
		t.Fatalf("unexpected error sending email: %v", err)
	}
}
