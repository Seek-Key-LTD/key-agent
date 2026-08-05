// Copyright 2026 Google LLC
package oracle_mem

import (
	"testing"
	"time"
)

func TestUnifiedContextManager_Struct(t *testing.T) {
	rec := CollectiveMemoryRecord{
		ID:        1,
		AgentID:   "Topaz_Seat_01",
		SeatName:  "Topaz",
		Category:  "CollectiveConsciousness",
		Content:   "August 5 2026 Collective Memory Test",
		Tags:      "test,adk,oracle26ai",
		CreatedAt: time.Now(),
	}

	if rec.SeatName != "Topaz" {
		t.Fatalf("expected Topaz, got %s", rec.SeatName)
	}
}
