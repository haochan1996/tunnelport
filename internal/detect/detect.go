// Package detect 实现环境自检（FR-1）与网络探测。
package detect

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"time"
)

// 黑名单（PRD §10.4）：绝不作为控制端口。
var blacklist = map[int]bool{
	22: true, 80: true, 443: true, 8000: true, 8080: true,
	8443: true, 8888: true, 5000: true, 5001: true, 3389: true,
}

const (
	candidateMin = 30000
	candidateMax = 40000
	maxTries     = 5
)

var ipProviders = []string{
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
}

// ExitIPv4 返回出口公网 IPv4。全部探测失败时返回错误。
func ExitIPv4(ctx context.Context) (string, error) {
	var lastErr error
	for _, u := range ipProviders {
		cctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		req, err := http.NewRequestWithContext(cctx, http.MethodGet, u, nil)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}
		buf := make([]byte, 64)
		n, _ := resp.Body.Read(buf)
		resp.Body.Close()
		cancel()
		ip := net.ParseIP(string(buf[:n]))
		if ip == nil || ip.To4() == nil {
			lastErr = fmt.Errorf("%s 返回的不是 IPv4", u)
			continue
		}
		return ip.String(), nil
	}
	return "", lastErr
}

var cgnatNet = net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// CGNATHint 检查本机网卡是否有 100.64.0.0/10 地址（FR-1 #3 的本地线索）。
func CGNATHint() bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
			if cgnatNet.Contains(ipn.IP) {
				return true
			}
		}
	}
	return false
}

// CanListen 测试本机能否监听 addr。
func CanListen(addr string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return l.Close()
}

// Listening 测试 127.0.0.1:port 是否有进程监听（doctor A 段 / FR-1 #6）。
func Listening(port int) bool {
	c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// TCPRtt 测量到 host:port 的 TCP 连接耗时（doctor B 段）。
func TCPRtt(host string, port int, timeout time.Duration) (time.Duration, error) {
	t0 := time.Now()
	c, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), timeout)
	if err != nil {
		return 0, err
	}
	c.Close()
	return time.Since(t0), nil
}

// NextFreePort 从 preferred 顺序向后找第一个可用端口（跳过黑名单），
// 最多向后探测 20 个；用于服务端口的冲突自动顺延（FR-8.2）。
// preferred <= 0 时返回 0（表示调用方自行决定默认值）。
// avoid 里的端口视为已占用（同一次启动内多个端口分配时防止互相撞车）。
func NextFreePort(preferred int, avoid ...int) int {
	if preferred <= 0 {
		return 0
	}
	skip := map[int]bool{}
	for _, a := range avoid {
		skip[a] = true
	}
	for i := 0; i < 20; i++ {
		cand := preferred + i
		if skip[cand] || blacklist[cand] || cand > 65535 {
			continue
		}
		if CanListen(net.JoinHostPort("0.0.0.0", strconv.Itoa(cand))) == nil {
			return cand
		}
	}
	return 0
}

// NextFreeBase 从 start 起，找连续 width 个可用端口的块起始值（逐个跳过黑名单、
// 逐个 CanListen 检查），最多滑移 20 次；用于"三连端口"整块分配（FR-2.3）。
// 找不到返回 0。
func NextFreeBase(start, width int) int {
	for i := 0; i < 20; i++ {
		base := start + i
		ok := true
		for j := 0; j < width; j++ {
			p := base + j
			if p > 65535 || blacklist[p] || CanListen(net.JoinHostPort("0.0.0.0", strconv.Itoa(p))) != nil {
				ok = false
				break
			}
		}
		if ok {
			return base
		}
	}
	return 0
}

// PickPort 选控制通道端口（PRD §10.4）。
// preferred > 0 时先试用户指定；否则在 30000–40000 随机试最多 5 个。
func PickPort(preferred int) (int, error) {
	try := func(p int) bool {
		if blacklist[p] {
			return false
		}
		return CanListen(net.JoinHostPort("0.0.0.0", strconv.Itoa(p))) == nil
	}
	if preferred > 0 {
		if !try(preferred) {
			return 0, fmt.Errorf("端口 %d 不可用（被占用或在黑名单）", preferred)
		}
		return preferred, nil
	}
	for i := 0; i < maxTries; i++ {
		p := candidateMin + rand.Intn(candidateMax-candidateMin+1)
		if try(p) {
			return p, nil
		}
	}
	return 0, fmt.Errorf("30000–40000 内连续 %d 个端口都不可用，请手动指定 --port", maxTries)
}
