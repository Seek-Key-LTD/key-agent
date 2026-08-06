// Copyright 2026 Google LLC
//
// Package feishu provides a direct Feishu Open API client with no external dependencies.

package feishu

import (
	"context"
	"encoding/json"
	"fmt"
)

// CalendarEvent represents a Feishu calendar event.
type CalendarEvent struct {
	Summary      string       `json:"summary"`
	Description  string       `json:"description"`
	StartTime    TimeInfo     `json:"start_time"`
	EndTime      TimeInfo     `json:"end_time"`
	Availability string       `json:"availability,omitempty"`
}

// TimeInfo holds the timestamp and timezone for an event time.
type TimeInfo struct {
	Timestamp string `json:"timestamp"`
	Timezone  string `json:"timezone"`
}

// createEventPayload is the request body for creating a calendar event.
type createEventPayload struct {
	Summary      string       `json:"summary"`
	Description  string       `json:"description"`
	StartTime    TimeInfo     `json:"start_time"`
	EndTime      TimeInfo     `json:"end_time"`
	Availability string       `json:"availability,omitempty"`
}

// createEventResponse is the response from the create event endpoint.
type createEventResponse struct {
	Code int                    `json:"code"`
	Msg  string                 `json:"msg"`
	Data map[string]interface{} `json:"data"`
}

// CreateEvent creates a calendar event in the specified calendar.
// Returns the event ID on success.
func (c *Client) CreateEvent(ctx context.Context, calendarID string, event CalendarEvent) (string, error) {
	payload := createEventPayload{
		Summary:      event.Summary,
		Description:  event.Description,
		StartTime:    event.StartTime,
		EndTime:      event.EndTime,
		Availability: event.Availability,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("feishu: marshal create event: %w", err)
	}

	path := fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events", calendarID)
	raw, err := c.Do(ctx, "POST", path, body)
	if err != nil {
		return "", fmt.Errorf("feishu: create event: %w", err)
	}

	var resp createEventResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("feishu: parse create event response: %w", err)
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("feishu: create event api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	eventID, _ := resp.Data["event_id"].(string)
	if eventID == "" {
		return "", fmt.Errorf("feishu: create event response missing event_id")
	}

	return eventID, nil
}