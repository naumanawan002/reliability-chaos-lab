package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type contextKey string

const CorrelationIDKey contextKey = "telemetry.correlation_id"

// MetricsCollector tracks service health counters and latency distributions.
type MetricsCollector struct {
	mu           sync.RWMutex
	TotalReqs    uint64
	SuccessReqs  uint64
	FailureReqs  uint64
	BreakerTrips uint64
	latencies    []time.Duration
}

func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		latencies: make([]time.Duration, 0, 1000),
	}
}

func (m *MetricsCollector) Record(duration time.Duration, isSuccess bool) {
	atomic.AddUint64(&m.TotalReqs, 1)
	if isSuccess {
		atomic.AddUint64(&m.SuccessReqs, 1)
	} else {
		atomic.AddUint64(&m.FailureReqs, 1)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.latencies) < 5000 {
		m.latencies = append(m.latencies, duration)
	}
}

func (m *MetricsCollector) RecordBreakerTrip() {
	atomic.AddUint64(&m.BreakerTrips, 1)
}

// ServeMetrics outputs Prometheus-compatible metrics text format.
func (m *MetricsCollector) ServeMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, "# HELP http_requests_total Total number of HTTP requests processed.\n")
	fmt.Fprintf(w, "# TYPE http_requests_total counter\n")
	fmt.Fprintf(w, "http_requests_total %d\n", atomic.LoadUint64(&m.TotalReqs))
	fmt.Fprintf(w, "http_requests_success_total %d\n", atomic.LoadUint64(&m.SuccessReqs))
	fmt.Fprintf(w, "http_requests_failure_total %d\n", atomic.LoadUint64(&m.FailureReqs))
	fmt.Fprintf(w, "circuit_breaker_trips_total %d\n", atomic.LoadUint64(&m.BreakerTrips))
}

// TracingMiddleware injects or forwards X-Correlation-ID across microservice hops.
func TracingMiddleware(collector *MetricsCollector, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		corrID := r.Header.Get("X-Correlation-ID")
		if corrID == "" {
			corrID = generateUUID()
		}

		w.Header().Set("X-Correlation-ID", corrID)
		ctx := context.WithValue(r.Context(), CorrelationIDKey, corrID)

		// Response capture
		recorder := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(recorder, r.WithContext(ctx))

		dur := time.Since(start)
		isSuccess := recorder.statusCode < 400
		collector.Record(dur, isSuccess)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rec *responseRecorder) WriteHeader(code int) {
	rec.statusCode = code
	rec.ResponseWriter.WriteHeader(code)
}

func generateUUID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
