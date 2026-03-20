package output

import (
	"testing"
	"time"
)

func TestNewResponse(t *testing.T) {
	r := NewResponse()
	if r.Ts == "" {
		t.Error("expected timestamp to be set")
	}
	if r.Results != nil {
		t.Errorf("expected nil results, got %v", r.Results)
	}
	if r.Failures != nil {
		t.Errorf("expected nil failures, got %v", r.Failures)
	}
	if r.Summary != nil {
		t.Errorf("expected nil summary, got %v", r.Summary)
	}
}

func TestAddResult(t *testing.T) {
	r := NewResponse()
	r.AddResult("item1")
	r.AddResult(42)
	if len(r.Results) != 2 {
		t.Errorf("want 2 results, got %d", len(r.Results))
	}
	if r.Results[0] != "item1" {
		t.Errorf("results[0]: want 'item1', got %v", r.Results[0])
	}
	if r.Results[1] != 42 {
		t.Errorf("results[1]: want 42, got %v", r.Results[1])
	}
}

func TestAddFailure(t *testing.T) {
	r := NewResponse()
	r.AddFailure("host1", "CONN_ERROR", "connection refused", "check SSH keys")
	if len(r.Failures) != 1 {
		t.Fatalf("want 1 failure, got %d", len(r.Failures))
	}
	f := r.Failures[0]
	if f.Host != "host1" {
		t.Errorf("host: want host1, got %s", f.Host)
	}
	if f.Code != "CONN_ERROR" {
		t.Errorf("code: want CONN_ERROR, got %s", f.Code)
	}
	if f.Message != "connection refused" {
		t.Errorf("message: want 'connection refused', got %s", f.Message)
	}
	if f.Suggestion != "check SSH keys" {
		t.Errorf("suggestion: want 'check SSH keys', got %s", f.Suggestion)
	}
}

func TestFinalize(t *testing.T) {
	r := NewResponse()
	r.AddResult("item1")
	r.AddResult("item2")
	r.AddFailure("h1", "ERR", "err msg", "")

	start := time.Now().Add(-100 * time.Millisecond)
	r.Finalize(start)

	if r.Summary == nil {
		t.Fatal("expected summary to be set")
	}
	if r.Summary.Total != 3 {
		t.Errorf("total: want 3, got %d", r.Summary.Total)
	}
	if r.Summary.Success != 2 {
		t.Errorf("success: want 2, got %d", r.Summary.Success)
	}
	if r.Summary.Failed != 1 {
		t.Errorf("failed: want 1, got %d", r.Summary.Failed)
	}
	if r.Duration <= 0 {
		t.Errorf("duration should be > 0, got %d", r.Duration)
	}
}

func TestFinalizeEmpty(t *testing.T) {
	r := NewResponse()
	r.Finalize(time.Now())
	if r.Summary != nil {
		t.Errorf("expected nil summary for empty response, got %+v", r.Summary)
	}
}

func TestFinalizeOnlyFailures(t *testing.T) {
	r := NewResponse()
	r.AddFailure("h1", "ERR", "msg", "")
	r.AddFailure("h2", "ERR", "msg", "")
	r.Finalize(time.Now())
	if r.Summary == nil {
		t.Fatal("expected summary")
	}
	if r.Summary.Total != 2 {
		t.Errorf("total: want 2, got %d", r.Summary.Total)
	}
	if r.Summary.Success != 0 {
		t.Errorf("success: want 0, got %d", r.Summary.Success)
	}
	if r.Summary.Failed != 2 {
		t.Errorf("failed: want 2, got %d", r.Summary.Failed)
	}
}
