package server

import "testing"

func TestRetrievalDiagnosticsKeepsRecentTracesAndLatencySummary(t *testing.T) {
	t.Parallel()

	d := newRetrievalDiagnostics()
	for i := 0; i < diagnosticsTraceLimit+2; i++ {
		d.recordTrace(retrievalTrace{
			Flow:      "check",
			Task:      "behavior",
			LatencyMs: int64(10 + i),
		})
	}

	_, traces, rows := d.snapshot()
	if len(traces) != diagnosticsTraceLimit {
		t.Fatalf("expected trace limit %d, got %d", diagnosticsTraceLimit, len(traces))
	}
	if traces[0].LatencyMs != int64(10+diagnosticsTraceLimit+1) {
		t.Fatalf("expected newest trace first, got %#v", traces[0])
	}
	if len(rows) != 2 {
		t.Fatalf("expected overall + task latency rows, got %#v", rows)
	}
	if rows[0].Name != "overall" {
		t.Fatalf("expected overall row first, got %#v", rows)
	}
	if rows[1].Name != "check:behavior" {
		t.Fatalf("expected task row, got %#v", rows[1])
	}
}
