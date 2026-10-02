package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"
)

type sample struct {
	PID          int     `json:"pid"`
	Elapsed      float64 `json:"elapsed_seconds"`
	Phase        string  `json:"phase"`
	HeapAlloc    uint64  `json:"heap_alloc_bytes"`
	HeapInuse    uint64  `json:"heap_inuse_bytes"`
	HeapIdle     uint64  `json:"heap_idle_bytes"`
	HeapReleased uint64  `json:"heap_released_bytes"`
	HeapSys      uint64  `json:"heap_sys_bytes"`
	Sys          uint64  `json:"runtime_sys_bytes"`
	NextGC       uint64  `json:"next_gc_bytes"`
	NumGC        uint32  `json:"gc_count"`
	Goroutines   int     `json:"goroutines"`
	Footprint    uint64  `json:"physical_footprint_bytes,omitempty"`
	NativeError  string  `json:"native_measurement_error,omitempty"`
}

func collect(ctx context.Context, directory string, duration time.Duration) error {
	file, err := os.OpenFile(filepath.Join(directory, "samples.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create samples: %w", err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	start := time.Now()
	write := func(phase string) error {
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		footprint, nativeErr := physicalFootprint(ctx)
		message := ""
		if nativeErr != nil {
			message = nativeErr.Error()
		}
		return encoder.Encode(sample{PID: os.Getpid(), Elapsed: time.Since(start).Seconds(), Phase: phase,
			HeapAlloc: stats.HeapAlloc, HeapInuse: stats.HeapInuse, HeapIdle: stats.HeapIdle,
			HeapReleased: stats.HeapReleased, HeapSys: stats.HeapSys, Sys: stats.Sys,
			NextGC: stats.NextGC, NumGC: stats.NumGC, Goroutines: runtime.NumGoroutine(), Footprint: footprint, NativeError: message})
	}
	if err := write("ready"); err != nil {
		return err
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	deadline := time.NewTimer(duration)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := write("idle"); err != nil {
				return err
			}
		case <-deadline.C:
			if err := write("before_gc"); err != nil {
				return err
			}
			// Probe only: collect unreachable objects to distinguish live allocations
			// from GC headroom. The released application does not force periodic GC.
			runtime.GC()
			if err := write("after_gc"); err != nil {
				return err
			}
			profile, err := os.OpenFile(filepath.Join(directory, "heap.pprof"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				return err
			}
			err = pprof.WriteHeapProfile(profile)
			closeErr := profile.Close()
			if err != nil {
				return fmt.Errorf("write heap profile: %w", err)
			}
			return closeErr
		}
	}
}
