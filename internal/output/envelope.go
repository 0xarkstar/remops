package output

import "time"

// Response is the standard envelope for all remops output.
type Response struct {
	Results  []any      `json:"results,omitempty"`
	Failures []HostError `json:"failures,omitempty"`
	Summary  *Summary   `json:"summary,omitempty"`
	Ts       string     `json:"timestamp"`
	Duration int64      `json:"duration_ms"`
}

// HostError represents a per-host failure.
type HostError struct {
	Host       string `json:"host"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

// Summary provides aggregate counts.
type Summary struct {
	Total   int `json:"total"`
	Success int `json:"success"`
	Failed  int `json:"failed"`
}

// NewResponse creates a new response with the current timestamp.
func NewResponse() *Response {
	return &Response{
		Ts: time.Now().UTC().Format(time.RFC3339),
	}
}

// AddResult appends a successful result.
func (r *Response) AddResult(result any) {
	r.Results = append(r.Results, result)
}

// AddFailure appends a host failure.
func (r *Response) AddFailure(host, code, message string, suggestion string) {
	r.Failures = append(r.Failures, HostError{
		Host:       host,
		Code:       code,
		Message:    message,
		Suggestion: suggestion,
	})
}

// Finalize sets the summary and duration.
func (r *Response) Finalize(startTime time.Time) {
	r.Duration = time.Since(startTime).Milliseconds()
	total := len(r.Results) + len(r.Failures)
	if total > 0 {
		r.Summary = &Summary{
			Total:   total,
			Success: len(r.Results),
			Failed:  len(r.Failures),
		}
	}
}
