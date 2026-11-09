package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestExecuteSuccess(t *testing.T) {
	ctx := context.Background()
	taskType := "email"
	payload := json.RawMessage(`{
		"to":"test@example.com",
		"simulate_failure":false
	}`)
	err := Execute(ctx, taskType, payload)
	if err != nil {
		t.Fatalf("execute failed:%v", err)
	}

}

func TestExecuteFailure(t *testing.T) {
	ctx := context.Background()
	taskType := "email"
	payload := json.RawMessage(`{
		"to":"test@example.com",
		"simulate_failure":true
	}`)
	err := Execute(ctx, taskType, payload)
	if err == nil {
		t.Fatalf("execute succeed")
	}
}

func TestExecuteInvalidJSON(t *testing.T) {
	ctx := context.Background()
	taskType := "email"
	payload := json.RawMessage(`{
    "to":"test@example.com"
    "simulate_failure":false
}`)
	err := Execute(ctx, taskType, payload)
	if err == nil {
		t.Fatalf("execute succeed")
	}

}
func TestExecuteUnknownType(t *testing.T) {
	ctx := context.Background()
	taskType := "unknown"
	payload := json.RawMessage(`{
	"to":"test@example.com",
	"simulate_failure":false
	}`)
	err := Execute(ctx, taskType, payload)
	if err == nil {
		t.Fatalf("execute succeed")
	}
}
func TestExecuteCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	taskType := "email"
	payload := json.RawMessage(`{
	"to":"test@example.com",
	"simulate_failure":false
	}`)
	cancel()
	err := Execute(ctx, taskType, payload)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}
