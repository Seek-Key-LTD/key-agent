// Copyright 2026 Google LLC
//
// CLI subcommands for Feishu integration.
// Enables pure Go in-process Feishu operations without external lark-cli.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"google.golang.org/adk/v2/feishu"
)

func handleFeishuCommand(args []string) bool {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printFeishuHelp()
		return true
	}

	subcmd := args[0]
	subArgs := args[1:]

	switch subcmd {
	case "status", "whoami":
		return runFeishuStatus(subArgs)
	case "im-send", "send":
		return runFeishuIMSend(subArgs)
	case "bitable-add":
		return runFeishuBitableAdd(subArgs)
	case "bitable-update":
		return runFeishuBitableUpdate(subArgs)
	case "calendar-add":
		return runFeishuCalendarAdd(subArgs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown feishu subcommand: %s\nRun 'k-agent feishu help' for usage.\n", subcmd)
		return true
	}
}

func printFeishuHelp() {
	fmt.Println(`KeyAgent Native Feishu CLI (Zero External Dependencies)

Usage:
  k-agent feishu <command> [options]

Available Commands:
  status, whoami     Check Feishu credentials and display current token status
  im-send, send      Send Feishu IM message to open_id, chat_id, or email
  bitable-add        Add a record to Feishu Bitable (Multi-dimensional table)
  bitable-update     Update a record in Feishu Bitable
  calendar-add       Create a calendar event

Authentication:
  Credentials are read from environment variables (e.g. injected by Secret Provider):
  - FEISHU_APP_ID (or LARK_APP_ID)
  - FEISHU_APP_SECRET (or LARK_APP_SECRET)

Examples:
  k-agent feishu status
  k-agent feishu im-send --to ou_xxx --text "Mission completed"
  k-agent feishu bitable-add --app bascnXXX --table tblXXX --fields '{"Name":"Alice","Score":100}'`)
}

func runFeishuStatus(args []string) bool {
	client, err := feishu.NewFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Feishu auth error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token, err := client.TenantAccessToken(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to obtain tenant_access_token: %v\n", err)
		os.Exit(1)
	}

	tokenPreview := token
	if len(token) > 16 {
		tokenPreview = token[:8] + "..." + token[len(token)-4:]
	}

	res := map[string]interface{}{
		"ok":       true,
		"status":   "ready",
		"app_id":   client.AppID(),
		"token":    tokenPreview,
		"cached":   true,
		"identity": "bot_tenant",
		"message":  "Feishu App credentials verified via internal Go SDK",
	}

	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))
	return true
}

func runFeishuIMSend(args []string) bool {
	fs := flag.NewFlagSet("im-send", flag.ExitOnError)
	to := fs.String("to", "", "Recipient ID (open_id, chat_id, user_id, or email)")
	recvType := fs.String("type", "open_id", "Recipient ID type: open_id | chat_id | user_id | email | union_id")
	text := fs.String("text", "", "Text message content")
	msgType := fs.String("msg-type", "text", "Message type (default: text)")
	_ = fs.Parse(args)

	if *to == "" || *text == "" {
		fmt.Fprintln(os.Stderr, "Error: --to and --text are required")
		fs.Usage()
		os.Exit(1)
	}

	client, err := feishu.NewFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Feishu auth error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var contentJSON string
	if *msgType == "text" {
		cBytes, _ := json.Marshal(map[string]string{"text": *text})
		contentJSON = string(cBytes)
	} else {
		contentJSON = *text
	}

	msgID, err := client.SendMessage(ctx, *to, *recvType, *msgType, contentJSON)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to send message: %v\n", err)
		os.Exit(1)
	}

	res := map[string]interface{}{
		"ok":         true,
		"message_id": msgID,
		"to":         *to,
		"type":       *recvType,
	}
	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))
	return true
}

func runFeishuBitableAdd(args []string) bool {
	fs := flag.NewFlagSet("bitable-add", flag.ExitOnError)
	appToken := fs.String("app", "", "Bitable app token")
	tableID := fs.String("table", "", "Bitable table ID")
	fieldsJSON := fs.String("fields", "", "JSON string representing row fields")
	_ = fs.Parse(args)

	if *appToken == "" || *tableID == "" || *fieldsJSON == "" {
		fmt.Fprintln(os.Stderr, "Error: --app, --table, and --fields are required")
		fs.Usage()
		os.Exit(1)
	}

	var fields map[string]interface{}
	if err := json.Unmarshal([]byte(*fieldsJSON), &fields); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid JSON in --fields: %v\n", err)
		os.Exit(1)
	}

	client, err := feishu.NewFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Feishu auth error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	recID, err := client.AddRecord(ctx, *appToken, *tableID, fields)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to add bitable record: %v\n", err)
		os.Exit(1)
	}

	res := map[string]interface{}{
		"ok":        true,
		"record_id": recID,
		"table_id":  *tableID,
	}
	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))
	return true
}

func runFeishuBitableUpdate(args []string) bool {
	fs := flag.NewFlagSet("bitable-update", flag.ExitOnError)
	appToken := fs.String("app", "", "Bitable app token")
	tableID := fs.String("table", "", "Bitable table ID")
	recordID := fs.String("record", "", "Bitable record ID")
	fieldsJSON := fs.String("fields", "", "JSON string representing row fields")
	_ = fs.Parse(args)

	if *appToken == "" || *tableID == "" || *recordID == "" || *fieldsJSON == "" {
		fmt.Fprintln(os.Stderr, "Error: --app, --table, --record, and --fields are required")
		fs.Usage()
		os.Exit(1)
	}

	var fields map[string]interface{}
	if err := json.Unmarshal([]byte(*fieldsJSON), &fields); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid JSON in --fields: %v\n", err)
		os.Exit(1)
	}

	client, err := feishu.NewFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Feishu auth error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.UpdateRecord(ctx, *appToken, *tableID, *recordID, fields); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to update bitable record: %v\n", err)
		os.Exit(1)
	}

	res := map[string]interface{}{
		"ok":        true,
		"record_id": *recordID,
		"status":    "updated",
	}
	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))
	return true
}

func runFeishuCalendarAdd(args []string) bool {
	fs := flag.NewFlagSet("calendar-add", flag.ExitOnError)
	summary := fs.String("summary", "", "Event summary")
	desc := fs.String("desc", "", "Event description")
	startStr := fs.String("start", "", "Start time RFC3339 (e.g. 2026-10-17T15:00:00+08:00)")
	durationMin := fs.Int("duration", 30, "Duration in minutes")
	_ = fs.Parse(args)

	if *summary == "" {
		fmt.Fprintln(os.Stderr, "Error: --summary is required")
		fs.Usage()
		os.Exit(1)
	}

	var startTime time.Time
	if *startStr != "" {
		var err error
		startTime, err = time.Parse(time.RFC3339, *startStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid start time: %v (must be RFC3339)\n", err)
			os.Exit(1)
		}
	} else {
		startTime = time.Now()
	}
	endTime := startTime.Add(time.Duration(*durationMin) * time.Minute)

	client, err := feishu.NewFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Feishu auth error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	eventID, err := client.CreateEvent(ctx, "primary", feishu.CalendarEvent{
		Summary:     *summary,
		Description: *desc,
		StartTime: feishu.TimeInfo{
			Timestamp: fmt.Sprintf("%d", startTime.Unix()),
		},
		EndTime: feishu.TimeInfo{
			Timestamp: fmt.Sprintf("%d", endTime.Unix()),
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create calendar event: %v\n", err)
		os.Exit(1)
	}

	res := map[string]interface{}{
		"ok":       true,
		"event_id": eventID,
		"summary":  *summary,
		"start":    startTime.Format(time.RFC3339),
		"end":      endTime.Format(time.RFC3339),
	}
	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))
	return true
}
