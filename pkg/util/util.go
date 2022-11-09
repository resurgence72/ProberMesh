package util

import (
	"context"
	"math/rand"
	"time"
)

const jitter = 50

// 设置随机延迟，防止并发探测量过大
func SetJitter() int {
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(jitter)
}

func Wait(ctx context.Context, interval time.Duration, f func()) {
	for {
		f()
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}
