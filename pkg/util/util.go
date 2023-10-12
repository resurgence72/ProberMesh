package util

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"probermesh/pkg/version"
)

const (
	jitter = 100

	ProjectName         = "probermesh"
	ProbeHTTPType       = "http"
	ProbeICMPType       = "icmp"
	defaultKeySeparator = string('\xfe')
)

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

func GetVersion() string {
	return version.Version
}

func GetMd5(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func WithJSONHeader(f func(r *http.Request) []byte) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(f(r))
	}
}

func SplitKey(key string) []string {
	return strings.Split(key, defaultKeySeparator)
}

func JoinKey(ks ...string) string {
	return strings.Join(ks, defaultKeySeparator)
}
