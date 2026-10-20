package modelroute

import "encoding/json"

const (
	// UTF-8 bytes are a conservative heuristic, not Jev token counts. Account for
	// all state and questions, and reserve space for the CLI/API envelope/model.
	MaxQuestionBytes     = 48 * 1024
	MaxRequestBytes      = 96 * 1024
	EnvelopeReserveBytes = 2 * 1024
)

type RequestBudget struct {
	Method               string `json:"method"`
	MaxQuestionBytes     int    `json:"max_question_bytes"`
	MaxRequestBytes      int    `json:"max_request_bytes"`
	EnvelopeReserveBytes int    `json:"envelope_reserve_bytes"`
	StateBytes           int    `json:"state_bytes"`
	QuestionBytes        int    `json:"question_bytes"`
	RequestBytes         int    `json:"request_bytes"`
	TruncatedFiles       int    `json:"truncated_files"`
	OmittedFiles         int    `json:"omitted_files"`
}

func measureRequest(in Input, questions map[string]any) *RequestBudget {
	b := &RequestBudget{Method: "serialized_utf8_bytes_heuristic", MaxQuestionBytes: MaxQuestionBytes, MaxRequestBytes: MaxRequestBytes, EnvelopeReserveBytes: EnvelopeReserveBytes}
	raw, _ := json.Marshal(in)
	b.StateBytes = len(raw)
	envelope := func(q any) int {
		raw, _ := json.Marshal(map[string]any{"state": in, "questions": q})
		return len(raw) + EnvelopeReserveBytes
	}
	b.RequestBytes = envelope(questions)
	for key, q := range questions {
		b.QuestionBytes = max(b.QuestionBytes, envelope(map[string]any{key: q}))
	}
	for _, f := range in.Files {
		if f.Truncated {
			b.TruncatedFiles++
		}
		if f.Omitted {
			b.OmittedFiles++
		}
	}
	return b
}
func (b *RequestBudget) fits() bool {
	return b.QuestionBytes <= b.MaxQuestionBytes && b.RequestBytes <= b.MaxRequestBytes
}

// FitRequest budgets the real shared state plus all/per-question envelopes.
// Fair prefix caps avoid consuming the entire allowance on the first file.
// Task, policy, candidates, quota and questions are never silently rewritten.
func FitRequest(in Input, questions map[string]any) (Input, *RequestBudget, error) {
	if _, err := json.Marshal(in); err != nil {
		return in, nil, NewFailure("invalid_request")
	}
	if _, err := json.Marshal(questions); err != nil {
		return in, nil, NewFailure("invalid_request")
	}
	in.Files = append([]TaskFile(nil), in.Files...)
	for i := range in.Files {
		f := &in.Files[i]
		if f.OriginalBytes == 0 {
			f.OriginalBytes = len(f.Content)
		}
		f.IncludedBytes = len(f.Content)
		if f.SourceBytes == 0 && !f.Truncated && f.Status == "attached" {
			f.SourceBytes = int64(len(f.Content))
		}
		if f.SourceSHA256 == "" && !f.Truncated && f.Status == "attached" {
			f.SourceSHA256 = f.SHA256
		}
	}
	b := measureRequest(in, questions)
	if b.fits() {
		return in, b, nil
	}
	capAll := func(cap int) Input {
		v := in
		v.Files = append([]TaskFile(nil), in.Files...)
		for i := range v.Files {
			v.Files[i] = clipFile(v.Files[i], cap, "request_budget")
		}
		return v
	}
	best := capAll(0)
	budget := measureRequest(best, questions)
	if !budget.fits() {
		return best, budget, NewFailure("context_too_large")
	}
	hi := 0
	for _, f := range in.Files {
		hi = max(hi, len(f.Content))
	}
	lo := 0
	for lo <= hi {
		mid := (lo + hi) / 2
		v := capAll(mid)
		b := measureRequest(v, questions)
		if b.fits() {
			best = v
			budget = b
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	return best, budget, nil
}

func reduceAttachments(in Input) (Input, bool) {
	in.Files = append([]TaskFile(nil), in.Files...)
	changed := false
	for i, f := range in.Files {
		if len(f.Content) > 0 {
			in.Files[i] = clipFile(f, len(f.Content)/2, "context_retry")
			changed = true
		}
	}
	return in, changed
}
