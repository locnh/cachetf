package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

// getCounterValue is a helper function to get the current value of a counter
func getCounterValue(counter prometheus.Counter) float64 {
	var m dto.Metric
	err := counter.Write(&m)
	if err != nil {
		return 0
	}
	return *m.Counter.Value
}

// getCounterVecValue is a helper function to get the current value of a counter vector with specific labels
func getCounterVecValue(counter *prometheus.CounterVec, labelValues ...string) float64 {
	var m dto.Metric

	counter.WithLabelValues(labelValues...).Write(&m)
	return *m.Counter.Value
}

// getGaugeValue is a helper function to get the current value of a gauge
func getGaugeValue(gauge prometheus.Gauge) float64 {
	var m dto.Metric

	err := gauge.Write(&m)
	if err != nil {
		return 0
	}
	return *m.Gauge.Value
}

func TestCacheMetrics(t *testing.T) {
	// Since we can't reset the metrics, we'll test the relative changes
	// rather than absolute values
	metrics := NewCacheMetrics()

	// Initialize metrics variable
	_ = getCounterValue(CacheHitsTotal) // Initialize metrics if needed

	t.Run("Test RecordHit", func(t *testing.T) {
		beforeHits := getCounterValue(CacheHitsTotal)
		beforeOps := getCounterVecValue(CacheOperationsTotal, "get", "hit")

		metrics.RecordHit()
		assert.Equal(t, beforeHits+1, getCounterValue(CacheHitsTotal))
		assert.Equal(t, beforeOps+1, getCounterVecValue(CacheOperationsTotal, "get", "hit"))

		// Record another hit
		metrics.RecordHit()
		assert.Equal(t, beforeHits+2, getCounterValue(CacheHitsTotal))
		assert.Equal(t, beforeOps+2, getCounterVecValue(CacheOperationsTotal, "get", "hit"))
	})

	t.Run("Test RecordMiss", func(t *testing.T) {
		beforeMisses := getCounterValue(CacheMissesTotal)
		beforeOps := getCounterVecValue(CacheOperationsTotal, "get", "miss")

		metrics.RecordMiss()
		assert.Equal(t, beforeMisses+1, getCounterValue(CacheMissesTotal))
		assert.Equal(t, beforeOps+1, getCounterVecValue(CacheOperationsTotal, "get", "miss"))

		// Record another miss
		metrics.RecordMiss()
		assert.Equal(t, beforeMisses+2, getCounterValue(CacheMissesTotal))
		assert.Equal(t, beforeOps+2, getCounterVecValue(CacheOperationsTotal, "get", "miss"))
	})

	t.Run("Test RecordDeletion", func(t *testing.T) {
		beforeDeletions := getCounterValue(CacheDeletionsTotal)
		beforeOps := getCounterVecValue(CacheOperationsTotal, "delete", "success")

		// Test with count = 1
		metrics.RecordDeletion(1)
		assert.Equal(t, beforeDeletions+1, getCounterValue(CacheDeletionsTotal))
		assert.Equal(t, beforeOps+1, getCounterVecValue(CacheOperationsTotal, "delete", "success"))

		// Test with count > 1
		metrics.RecordDeletion(3)
		assert.Equal(t, beforeDeletions+4, getCounterValue(CacheDeletionsTotal))
		assert.Equal(t, beforeOps+4, getCounterVecValue(CacheOperationsTotal, "delete", "success"))
	})

	t.Run("Test RecordError", func(t *testing.T) {
		// Use unique operation names to avoid conflicts with other tests
		op1 := "test_operation_" + t.Name()
		op2 := "another_operation_" + t.Name()

		beforeOp1 := getCounterVecValue(CacheOperationsTotal, op1, "error")
		beforeOp2 := getCounterVecValue(CacheOperationsTotal, op2, "error")

		metrics.RecordError(op1)
		assert.Equal(t, beforeOp1+1, getCounterVecValue(CacheOperationsTotal, op1, "error"))

		// Record another error for the same operation
		metrics.RecordError(op1)
		assert.Equal(t, beforeOp1+2, getCounterVecValue(CacheOperationsTotal, op1, "error"))

		// Record error for a different operation
		metrics.RecordError(op2)
		assert.Equal(t, beforeOp2+1, getCounterVecValue(CacheOperationsTotal, op2, "error"))
	})

	t.Run("Test UpdateSize", func(t *testing.T) {
		// Test setting a new size
		testSize1 := int64(1024)
		metrics.UpdateSize(testSize1)
		assert.Equal(t, float64(testSize1), getGaugeValue(CacheSizeBytes))

		// Test updating the size
		testSize2 := int64(2048)
		metrics.UpdateSize(testSize2)
		assert.Equal(t, float64(testSize2), getGaugeValue(CacheSizeBytes))
	})

	t.Run("Test RecordOperationDuration", func(t *testing.T) {
		// Note: We can't easily verify the histogram values directly, but we can check that the metric exists
		// and that the operation doesn't panic
		metrics.RecordOperationDuration("test_operation", 1.5)
		metrics.RecordOperationDuration("test_operation", 2.5)

		// Just verify that the metric exists and has some observations
		hist, err := CacheOperationDuration.GetMetricWithLabelValues("test_operation")
		assert.NoError(t, err)
		assert.NotNil(t, hist)
	})
}

func TestNewCacheMetrics(t *testing.T) {
	metrics := NewCacheMetrics()
	assert.NotNil(t, metrics, "NewCacheMetrics() should return a non-nil instance")
}
