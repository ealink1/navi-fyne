package main

import (
	"context"
	"sync"
)

// 本文件承载 driver-agent 的在途查询取消通道。
//
// 背景：在没有取消通道时，主进程只能杀掉 agent 进程来停止查询——服务端批处理仍在
// 继续，且该连接随即失效。这里让 agent 为当前业务请求登记一个可取消 context，主进程
// 发出的取消通知命中登记项后取消该 context，驱动层据此向服务端发送终止信号
// （如 SQL Server 的 TDS attention），连接保持可用。

// agentRequestCanceller 记录当前在途业务请求的取消入口。
//
// agent 以单 worker 串行执行业务请求，同一时刻最多只有一个在途请求，因此登记项无需
// 按请求 ID 建表；generation 沿用主进程 query_registry 的世代号思路，避免已完成请求
// 的收尾误删下一个请求的登记。
type agentRequestCanceller struct {
	mu             sync.Mutex
	nextGeneration int64
	requestID      int64
	generation     int64
	cancel         context.CancelFunc
}

// register 登记在途请求，返回供 unregister 校验的世代号。
func (c *agentRequestCanceller) register(requestID int64, cancel context.CancelFunc) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextGeneration++
	c.requestID = requestID
	c.generation = c.nextGeneration
	c.cancel = cancel
	return c.generation
}

func (c *agentRequestCanceller) unregister(requestID int64, generation int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation != generation || c.requestID != requestID {
		return
	}
	c.requestID = 0
	c.cancel = nil
}

// cancelRequest 取消指定请求。
//
// 目标不是当前在途请求时返回 false：迟到的取消通知绝不能中止其后才开始的新请求，
// 由主进程的宽限期兜底终止传输。
func (c *agentRequestCanceller) cancelRequest(requestID int64) bool {
	if requestID <= 0 {
		return false
	}
	c.mu.Lock()
	if c.requestID != requestID || c.cancel == nil {
		c.mu.Unlock()
		return false
	}
	cancel := c.cancel
	c.mu.Unlock()

	cancel()
	return true
}
