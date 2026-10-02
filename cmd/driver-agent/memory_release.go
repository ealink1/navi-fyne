package main

import (
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"time"
)

// 本文件集中管理 driver-agent 进程内的响应内存回收：大批量查询响应
// 解码后通过异步 GC 释放，避免常驻 RSS 峰值。

const (
	agentMemoryTrimRowsThreshold = 100000
	agentMemoryTrimMinInterval   = 3 * time.Second
)

var (
	agentMemoryTrimRunning  atomic.Bool
	agentMemoryTrimLastAt   atomic.Int64
	runAgentMemoryTrimAsync = func(fn func()) {
		go fn()
	}
	agentMemoryTrimFn = func() {
		runtime.GC()
		debug.FreeOSMemory()
	}
)

func countAgentResponseRows(data interface{}) int64 {
	rows, ok := data.([]map[string]interface{})
	if !ok {
		return 0
	}
	return int64(len(rows))
}

func maybeReleaseAgentMemory(reason string, rows int64) {
	if rows < agentMemoryTrimRowsThreshold {
		return
	}
	if !agentMemoryTrimRunning.CompareAndSwap(false, true) {
		return
	}

	runAgentMemoryTrimAsync(func() {
		defer agentMemoryTrimRunning.Store(false)
		if delay := nextAgentMemoryTrimDelay(); delay > 0 {
			time.Sleep(delay)
		}
		agentMemoryTrimFn()
		agentMemoryTrimLastAt.Store(time.Now().UnixNano())
	})
}

func nextAgentMemoryTrimDelay() time.Duration {
	lastUnixNano := agentMemoryTrimLastAt.Load()
	if lastUnixNano <= 0 {
		return 0
	}
	elapsed := time.Since(time.Unix(0, lastUnixNano))
	if elapsed >= agentMemoryTrimMinInterval {
		return 0
	}
	return agentMemoryTrimMinInterval - elapsed
}
