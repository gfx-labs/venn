package latencyhist

import (
	"sync"
	"time"
)

// LatencyHist stores a fixed-size circular buffer of latency measurements
type LatencyHist struct {
	mu       sync.RWMutex
	buffer   []time.Duration
	capacity int
	size     int
	head     int
	sum      time.Duration
}

// New creates a new LatencyHist with the specified capacity
func New(capacity int) *LatencyHist {
	if capacity <= 0 {
		capacity = 1
	}
	return &LatencyHist{
		buffer:   make([]time.Duration, capacity),
		capacity: capacity,
		size:     0,
		head:     0,
		sum:      0,
	}
}

// Add adds a new latency measurement to the histogram
func (h *LatencyHist) Add(latency time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// If buffer is full, subtract the value we're about to overwrite from sum
	if h.size == h.capacity {
		h.sum -= h.buffer[h.head]
	} else {
		h.size++
	}

	// Add new value
	h.buffer[h.head] = latency
	h.sum += latency
	h.head = (h.head + 1) % h.capacity
}

// Average returns the average latency of all stored measurements
func (h *LatencyHist) Average() time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.size == 0 {
		return 0
	}
	return h.sum / time.Duration(h.size)
}

// Min returns the minimum latency in the buffer
func (h *LatencyHist) Min() time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.size == 0 {
		return 0
	}

	min := time.Duration(1<<63 - 1) // max duration value
	for i := 0; i < h.size; i++ {
		if h.buffer[i] < min {
			min = h.buffer[i]
		}
	}
	return min
}

// Max returns the maximum latency in the buffer
func (h *LatencyHist) Max() time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.size == 0 {
		return 0
	}

	var max time.Duration
	for i := 0; i < h.size; i++ {
		if h.buffer[i] > max {
			max = h.buffer[i]
		}
	}
	return max
}

// Count returns the number of measurements currently stored
func (h *LatencyHist) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.size
}

// Clear removes all measurements from the histogram
func (h *LatencyHist) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.size = 0
	h.head = 0
	h.sum = 0
}

// Stats returns all statistics at once to avoid multiple lock operations
type Stats struct {
	Average time.Duration
	Min     time.Duration
	Max     time.Duration
	Count   int
}

// GetStats returns all statistics in a single call
func (h *LatencyHist) GetStats() Stats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.size == 0 {
		return Stats{}
	}

	stats := Stats{
		Count:   h.size,
		Average: h.sum / time.Duration(h.size),
		Min:     time.Duration(1<<63 - 1),
		Max:     0,
	}

	for i := 0; i < h.size; i++ {
		if h.buffer[i] < stats.Min {
			stats.Min = h.buffer[i]
		}
		if h.buffer[i] > stats.Max {
			stats.Max = h.buffer[i]
		}
	}

	return stats
}