package streaming

import "fmt"

// StreamReplayReport summarizes the result of replaying a stream.
type StreamReplayReport struct {
	StreamID        string `json:"stream_id"`
	TenantID        string `json:"tenant_id"`
	EventCount      int    `json:"event_count"`
	Divergences     []string `json:"divergences"`
	FinalSequence   int64  `json:"final_sequence"`
	IsDivergenceFree bool  `json:"is_divergence_free"`
}

// ReplayStream verifies hash integrity and sequence monotonicity for all events
// in the stream. It never halts on a bad event — all violations are recorded.
func ReplayStream(s *ExecutionStream) StreamReplayReport {
	events := s.Events()
	report := StreamReplayReport{
		StreamID: s.StreamID(),
		TenantID: s.TenantID(),
		EventCount: len(events),
	}

	var lastSeq int64
	for i, e := range events {
		if ok, reason := VerifyStreamEventHash(e); !ok {
			report.Divergences = append(report.Divergences,
				fmt.Sprintf("event[%d] %q hash invalid: %s", i, e.EventID, reason))
		}
		if e.Sequence <= lastSeq && i > 0 {
			report.Divergences = append(report.Divergences,
				fmt.Sprintf("event[%d] %q sequence %d not monotonically increasing (last=%d)",
					i, e.EventID, e.Sequence, lastSeq))
		}
		if e.Sequence > 0 {
			lastSeq = e.Sequence
		}
	}

	report.FinalSequence = lastSeq
	report.IsDivergenceFree = len(report.Divergences) == 0
	return report
}

// StreamSnapshot is a tamper-evident capture of stream state for archival.
type StreamSnapshot struct {
	StreamID    string       `json:"stream_id"`
	TenantID    string       `json:"tenant_id"`
	EventCount  int          `json:"event_count"`
	FinalSeq    int64        `json:"final_seq"`
	Events      []StreamEvent `json:"events"`
}

// BuildStreamSnapshot captures all events from a stream into a snapshot for replay.
func BuildStreamSnapshot(s *ExecutionStream) StreamSnapshot {
	events := s.Events()
	var finalSeq int64
	if len(events) > 0 {
		finalSeq = events[len(events)-1].Sequence
	}
	return StreamSnapshot{
		StreamID:   s.StreamID(),
		TenantID:   s.TenantID(),
		EventCount: len(events),
		FinalSeq:   finalSeq,
		Events:     events,
	}
}
