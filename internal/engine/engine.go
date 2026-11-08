// Package engine 定义隧道引擎抽象与状态机（PRD §10.3 / §10.5）。
// rathole 是当前唯一实现；Engine 接口是将来换引擎的逃生舱（D-1）。
package engine

import (
	"context"
	"time"
)

// State 状态机：init → connecting → connected → degraded → reconnecting → failed。
type State string

const (
	Init         State = "init"
	Connecting   State = "connecting"
	Connected    State = "connected"
	Degraded     State = "degraded"
	Reconnecting State = "reconnecting"
	Failed       State = "failed"
)

// Status 是引擎对外的只读快照。failed 必须携带 Reason 与 Hint（§10.5）。
type Status struct {
	State  State
	Reason string
	Hint   string
	Uptime time.Duration
}

// Service 是一条端口映射。
type Service struct {
	Name       string // 两侧必须一致
	LocalAddr  string // connect 端：本机目标地址
	PublicPort int    // serve 端：对外暴露的端口
}

// Config 是引擎启动参数。
type Config struct {
	Role        string // serve | connect
	Bind        string // serve 端控制通道监听地址
	ControlPort int    // serve 端控制通道端口
	Remote      string // connect 端：中继地址 host
	RemotePort  int    // connect 端：中继控制端口
	Token       string
	Services    []Service
	RatholeBin  string
	TempDir     string // 留空则自动建临时目录（FR-2.1：进程退出即清理）

	// Transport: "tls"（默认，D-8 链路加密）或 "tcp"（对端旧版本回退）。
	// tls 时 serve 需要 CertP12（PKCS#12），connect 需要 CertRoot（受信根 PEM）。
	Transport string
	CertP12   string
	CertRoot  string
}

// Engine 隧道引擎接口。
type Engine interface {
	// Start 拉起引擎进程并立即返回；内部 goroutine 负责保活与重连。
	Start(ctx context.Context, cfg Config) error
	Stop() error
	Status() Status
}
