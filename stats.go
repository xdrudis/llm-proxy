package main

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/montanaflynn/stats"
)

var (
	requestsTotal           atomic.Int64
	requestsSuccessful      atomic.Int64
	requestsFailed          atomic.Int64
	batchesTotal            atomic.Int64
	batchesSuccessful       atomic.Int64
	batchesFailed           atomic.Int64
	synthesizedErrResponses atomic.Int64

	requestTimings     []float64
	requestTimingsLock sync.Mutex

	batchTimings     []float64
	batchTimingsLock sync.Mutex
)

type Stats struct {
	Requests struct {
		Total                   int64   `json:"total"`
		Successful              int64   `json:"successful"`
		Failed                  int64   `json:"failed"`
		SynthesizedErrResponses int64   `json:"synthesized_error_responses"`
		AvgTime                 float64 `json:"avg_time_ms"`
		P50Time                 float64 `json:"p50_time_ms"`
		P95Time                 float64 `json:"p95_time_ms"`
		P99Time                 float64 `json:"p99_time_ms"`
	} `json:"requests"`
	Batches struct {
		Total      int64   `json:"total"`
		Successful int64   `json:"successful"`
		Failed     int64   `json:"failed"`
		AvgTime    float64 `json:"avg_time_ms"`
		P50Time    float64 `json:"p50_time_ms"`
		P95Time    float64 `json:"p95_time_ms"`
		P99Time    float64 `json:"p99_time_ms"`
	} `json:"batches"`
}

func trackRequestStart() {
	requestsTotal.Add(1)
}

func trackRequestEnd(success bool, duration time.Duration) {
	if success {
		requestsSuccessful.Add(1)
	} else {
		requestsFailed.Add(1)
	}

	requestTimingsLock.Lock()
	requestTimings = append(requestTimings, float64(duration.Milliseconds()))
	requestTimingsLock.Unlock()
}

func trackBatchStart() {
	batchesTotal.Add(1)
}

func trackBatchEnd(success bool, duration time.Duration) {
	if success {
		batchesSuccessful.Add(1)
	} else {
		batchesFailed.Add(1)
	}

	batchTimingsLock.Lock()
	batchTimings = append(batchTimings, float64(duration.Milliseconds()))
	batchTimingsLock.Unlock()
}

func trackSynthesizedErrorResponse() {
	synthesizedErrResponses.Add(1)
}

func getStats() Stats {
	var s Stats

	s.Requests.Total = requestsTotal.Load()
	s.Requests.Successful = requestsSuccessful.Load()
	s.Requests.Failed = requestsFailed.Load()
	s.Requests.SynthesizedErrResponses = synthesizedErrResponses.Load()

	s.Batches.Total = batchesTotal.Load()
	s.Batches.Successful = batchesSuccessful.Load()
	s.Batches.Failed = batchesFailed.Load()

	requestTimingsLock.Lock()
	if len(requestTimings) > 0 {
		s.Requests.AvgTime, _ = stats.Mean(requestTimings)
		s.Requests.P50Time, _ = stats.Percentile(requestTimings, 50)
		s.Requests.P95Time, _ = stats.Percentile(requestTimings, 95)
		s.Requests.P99Time, _ = stats.Percentile(requestTimings, 99)
	}
	requestTimingsLock.Unlock()

	batchTimingsLock.Lock()
	if len(batchTimings) > 0 {
		s.Batches.AvgTime, _ = stats.Mean(batchTimings)
		s.Batches.P50Time, _ = stats.Percentile(batchTimings, 50)
		s.Batches.P95Time, _ = stats.Percentile(batchTimings, 95)
		s.Batches.P99Time, _ = stats.Percentile(batchTimings, 99)
	}
	batchTimingsLock.Unlock()

	return s
}
