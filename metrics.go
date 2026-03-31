package main

import (
	"sync"
	"sync/atomic"
	"time"
)

// ── Metrics collector ─────────────────────────────────────────────────────────
// Tracks query timing stats for the hot endpoint (/inventory-by-location).
// Thread-safe — updated on every request, read by /metrics handler.
// Harness CV polls /metrics every 30s during the Verify step.

type MetricsStore struct {
	mu           sync.RWMutex
	totalQueries int64
	totalErrors  int64
	totalMs      int64
	p95Samples   []int64 // rolling window of last 100 response times
	windowStart  time.Time
}

var metrics = &MetricsStore{
	windowStart: time.Now(),
	p95Samples:  make([]int64, 0, 100),
}

func (m *MetricsStore) Record(durationMs int64, err bool) {
	atomic.AddInt64(&m.totalQueries, 1)
	atomic.AddInt64(&m.totalMs, durationMs)
	if err {
		atomic.AddInt64(&m.totalErrors, 1)
	}

	m.mu.Lock()
	m.p95Samples = append(m.p95Samples, durationMs)
	if len(m.p95Samples) > 100 {
		m.p95Samples = m.p95Samples[1:]
	}
	m.mu.Unlock()
}

func (m *MetricsStore) Snapshot() MetricsResponse {
	total := atomic.LoadInt64(&m.totalQueries)
	totalMs := atomic.LoadInt64(&m.totalMs)
	errors := atomic.LoadInt64(&m.totalErrors)

	var avg int64
	if total > 0 {
		avg = totalMs / total
	}

	m.mu.RLock()
	p95 := calcP95(m.p95Samples)
	m.mu.RUnlock()

	return MetricsResponse{
		TotalQueries:  total,
		TotalErrors:   errors,
		AvgResponseMs: avg,
		P95ResponseMs: p95,
		TimestampMs:   time.Now().UnixMilli(),
	}
}

func calcP95(samples []int64) int64 {
	if len(samples) == 0 {
		return 0
	}
	// simple sort-based p95
	sorted := make([]int64, len(samples))
	copy(sorted, samples)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	idx := int(float64(len(sorted)) * 0.95)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
