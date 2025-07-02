package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // CacheHitsTotal is a counter for cache hits
    CacheHitsTotal = promauto.NewCounter(prometheus.CounterOpts{
        Name: "cache_hits_total",
        Help: "Total number of cache hits",
    })

    // CacheMissesTotal is a counter for cache misses
    CacheMissesTotal = promauto.NewCounter(prometheus.CounterOpts{
        Name: "cache_misses_total",
        Help: "Total number of cache misses",
    })

    // CacheDeletionsTotal is a counter for cache deletions
    CacheDeletionsTotal = promauto.NewCounter(prometheus.CounterOpts{
        Name: "cache_deletions_total",
        Help: "Total number of cache deletions",
    })

    // CacheSizeBytes is a gauge for current cache size in bytes
    CacheSizeBytes = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "cache_size_bytes",
        Help: "Current size of the cache in bytes",
    })

    // CacheOperationsTotal is a counter for all cache operations
    CacheOperationsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "cache_operations_total",
            Help: "Total number of cache operations by type and status",
        },
        []string{"operation", "status"},
    )

    // CacheOperationDuration tracks the duration of cache operations
    CacheOperationDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "cache_operation_duration_seconds",
            Help:    "Time taken to process cache operations",
            Buckets: prometheus.DefBuckets,
        },
        []string{"operation"},
    )
)

// CacheMetrics wraps all cache-related metrics
type CacheMetrics struct {
    mu sync.Mutex
}

// NewCacheMetrics creates a new CacheMetrics instance
func NewCacheMetrics() *CacheMetrics {
    return &CacheMetrics{}
}

// RecordHit increments the cache hit counter
func (m *CacheMetrics) RecordHit() {
    CacheHitsTotal.Inc()
    CacheOperationsTotal.WithLabelValues("get", "hit").Inc()
}

// RecordMiss increments the cache miss counter
func (m *CacheMetrics) RecordMiss() {
    CacheMissesTotal.Inc()
    CacheOperationsTotal.WithLabelValues("get", "miss").Inc()
}

// RecordDeletion increments the deletion counter
func (m *CacheMetrics) RecordDeletion(count int) {
    CacheDeletionsTotal.Add(float64(count))
    CacheOperationsTotal.WithLabelValues("delete", "success").Add(float64(count))
}

// RecordError records an error for an operation
func (m *CacheMetrics) RecordError(operation string) {
    CacheOperationsTotal.WithLabelValues(operation, "error").Inc()
}

// RecordOperationDuration records the duration of an operation
func (m *CacheMetrics) RecordOperationDuration(operation string, duration float64) {
    CacheOperationDuration.WithLabelValues(operation).Observe(duration)
}

// UpdateSize updates the cache size gauge
func (m *CacheMetrics) UpdateSize(size int64) {
    CacheSizeBytes.Set(float64(size))
}
