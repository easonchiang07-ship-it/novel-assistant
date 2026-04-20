package server

import (
	"sort"
	"strings"
	"sync"
	"time"
)

const diagnosticsTraceLimit = 12

type reindexStatus struct {
	StartedAt   time.Time
	FinishedAt  time.Time
	DurationMs  int64
	Success     bool
	Error       string
	Characters  int
	Worlds      int
	Styles      int
	Chapters    int
	VectorCount int
}

type retrievalTrace struct {
	RecordedAt  time.Time
	Flow        string
	Task        string
	ChapterFile string
	ChapterName string
	IndexReady  bool
	LatencyMs   int64
	ResultCount int
	Error       string
	Config      retrievalSummary
	Sources     []referenceSummary
}

type latencyAggregate struct {
	Count        int
	SuccessCount int
	FailureCount int
	MinMs        int64
	MaxMs        int64
	TotalMs      int64
	LastMs       int64
}

type latencySummaryRow struct {
	Name         string
	Count        int
	SuccessCount int
	FailureCount int
	AvgMs        int64
	MinMs        int64
	MaxMs        int64
	LastMs       int64
}

type retrievalDiagnostics struct {
	mu           sync.RWMutex
	lastReindex  reindexStatus
	recentTraces []retrievalTrace
	latency      map[string]*latencyAggregate
}

func newRetrievalDiagnostics() *retrievalDiagnostics {
	return &retrievalDiagnostics{
		latency: make(map[string]*latencyAggregate),
	}
}

func (d *retrievalDiagnostics) recordReindex(status reindexStatus) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.lastReindex = status
	d.mu.Unlock()
}

func (d *retrievalDiagnostics) recordTrace(trace retrievalTrace) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	d.recentTraces = append([]retrievalTrace{trace}, d.recentTraces...)
	if len(d.recentTraces) > diagnosticsTraceLimit {
		d.recentTraces = d.recentTraces[:diagnosticsTraceLimit]
	}

	keys := []string{"overall", trace.Flow + ":" + trace.Task}
	for _, key := range keys {
		agg := d.latency[key]
		if agg == nil {
			agg = &latencyAggregate{MinMs: trace.LatencyMs, MaxMs: trace.LatencyMs}
			d.latency[key] = agg
		}
		agg.Count++
		agg.TotalMs += trace.LatencyMs
		agg.LastMs = trace.LatencyMs
		if trace.LatencyMs < agg.MinMs {
			agg.MinMs = trace.LatencyMs
		}
		if trace.LatencyMs > agg.MaxMs {
			agg.MaxMs = trace.LatencyMs
		}
		if strings.TrimSpace(trace.Error) == "" {
			agg.SuccessCount++
		} else {
			agg.FailureCount++
		}
	}
}

func (d *retrievalDiagnostics) snapshot() (reindexStatus, []retrievalTrace, []latencySummaryRow) {
	if d == nil {
		return reindexStatus{}, nil, nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()

	traces := make([]retrievalTrace, len(d.recentTraces))
	copy(traces, d.recentTraces)

	rows := make([]latencySummaryRow, 0, len(d.latency))
	for key, agg := range d.latency {
		if agg == nil || agg.Count == 0 {
			continue
		}
		rows = append(rows, latencySummaryRow{
			Name:         key,
			Count:        agg.Count,
			SuccessCount: agg.SuccessCount,
			FailureCount: agg.FailureCount,
			AvgMs:        agg.TotalMs / int64(agg.Count),
			MinMs:        agg.MinMs,
			MaxMs:        agg.MaxMs,
			LastMs:       agg.LastMs,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Name == "overall" {
			return true
		}
		if rows[j].Name == "overall" {
			return false
		}
		return rows[i].Name < rows[j].Name
	})

	return d.lastReindex, traces, rows
}

func (s *Server) recordRetrievalTrace(flow string, summary retrievalSummary, chapterTitle, chapterFile string, refs []vectorProfile, err error, duration time.Duration) {
	if s == nil || s.diagnostics == nil {
		return
	}
	title := strings.TrimSpace(chapterTitle)
	if title == "" && chapterFile != "" {
		title = strings.TrimSuffix(chapterFile, ".md")
	}
	trace := retrievalTrace{
		RecordedAt:  time.Now(),
		Flow:        flow,
		Task:        summary.Task,
		ChapterFile: strings.TrimSpace(chapterFile),
		ChapterName: title,
		IndexReady:  s.store != nil && s.store.Len() > 0,
		LatencyMs:   duration.Milliseconds(),
		ResultCount: len(refs),
		Config:      summary,
		Sources:     summarizeReferences(limitTraceReferences(refs, 3)),
	}
	if err != nil {
		trace.Error = err.Error()
	}
	s.diagnostics.recordTrace(trace)
}

func limitTraceReferences(items []vectorProfile, max int) []vectorProfile {
	if len(items) <= max {
		return items
	}
	return items[:max]
}
