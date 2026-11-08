// rathole 引擎实现：生成临时配置 → 拉起子进程 → 崩溃退避重启（FR-2.1 / FR-4.3）。
package engine

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/haochan1996/tunnelport/internal/certs"
)

// rathole 侧固定的服务名，serve 与 connect 两侧必须一致。
const (
	ServiceOrca   = "orca"
	ServiceHealth = "tp-health"
)

// HealthPort 健康端点在两侧使用的端口（FR-7.3 / D-3）。
const HealthPort = 6769

type rathole struct {
	mu      sync.Mutex
	cfg     Config
	status  Status
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	tmpDir  string
	logPath string
	started time.Time
	stopped bool
}

// NewRathole 返回 rathole 引擎实例。
func NewRathole() *rathole { return &rathole{} }

func (r *rathole) Start(ctx context.Context, cfg Config) error {
	if cfg.RatholeBin == "" {
		cfg.RatholeBin = "rathole"
	}
	tmp := cfg.TempDir
	if tmp == "" {
		d, err := os.MkdirTemp("", "tunnelport-*")
		if err != nil {
			return fmt.Errorf("创建临时目录失败: %w", err)
		}
		tmp = d
	}
	r.cfg = cfg
	r.tmpDir = tmp

	cfgPath := filepath.Join(tmp, "rathole.toml")
	if err := os.WriteFile(cfgPath, []byte(renderConfig(&cfg)), 0o600); err != nil {
		return fmt.Errorf("写配置失败: %w", err)
	}
	// 引擎日志落盘：子进程报错必须可追查（2026-11-07 冒烟测试教训）。
	// 首选状态目录（doctor 可读），退回临时目录。
	r.logPath = filepath.Join(tmp, "engine.log")
	if d, err := os.UserHomeDir(); err == nil {
		if _, err := os.Stat(filepath.Join(d, ".tunnelport")); err == nil {
			r.logPath = filepath.Join(d, ".tunnelport", "engine.log")
		}
	}

	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	go r.loop(runCtx, cfgPath)
	return nil
}

// maxEngineLog 引擎日志单文件上限（FR-9.1 轮转的简化版：超限滚动为 .old）。
const maxEngineLog = 5 << 20 // 5 MB

// engineLog 打开日志文件（追加）；超过上限时滚动为 engine.log.old，防止无限增长
// （2026-11-08 稳定性审查 #4）。
func (r *rathole) engineLog() (*os.File, error) {
	if st, err := os.Stat(r.logPath); err == nil && st.Size() > maxEngineLog {
		_ = os.Rename(r.logPath, r.logPath+".old")
	}
	f, err := os.OpenFile(r.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// tailLog 取日志最后 n 行（失败时进 Reason，让用户看得懂）。
func (r *rathole) tailLog(n int) string {
	b, err := os.ReadFile(r.logPath)
	if err != nil {
		return ""
	}
	lines := bytes.Split(bytes.TrimRight(b, "\n"), []byte("\n"))
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	out := make([][]byte, len(lines))
	for i, l := range lines {
		out[i] = l
	}
	return string(bytes.Join(out, []byte(" | ")))
}

// loop 是保活循环：进程退出且未被取消时，按 1→2→4→8→16→30s ±20% 抖动重启。
func (r *rathole) loop(ctx context.Context, cfgPath string) {
	// PDEATHSIG 绑定在创建子进程的 OS 线程上（Linux）；Go 运行时回收闲置线程会
	// 误杀健康的子进程（golang/go#27505）。锁定线程后，信号绑定的线程与本
	// goroutine 同寿命，行为才正确（2026-11-08 稳定性审查实测发现）。
	runtime.LockOSThread()
	backoffs := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second}
	attempt := 0
	for {
		if ctx.Err() != nil {
			return
		}
		set := func(s State, reason, hint string) {
			r.mu.Lock()
			r.status = Status{State: s, Reason: reason, Hint: hint, Uptime: since(r.started)}
			r.mu.Unlock()
		}
		cmd := exec.CommandContext(ctx, r.cfg.RatholeBin, cfgPath)
		setPDeathsig(cmd)
		f, ferr := r.engineLog()
		if ferr == nil {
			cmd.Stdout, cmd.Stderr = f, f
		}
		r.mu.Lock()
		r.cmd = cmd
		r.mu.Unlock()
		if attempt == 0 {
			r.started = time.Now()
		}
		set(Connecting, "", "")
		t0 := time.Now()
		if err := cmd.Start(); err != nil {
			set(Failed, "引擎启动失败: "+err.Error(),
				"确认 rathole 已安装且可执行，或用 --rathole 指定路径")
			return
		}
		waitErr := make(chan error, 1)
		go func() { waitErr <- cmd.Wait() }()

		// 连续存活 3s 视为已建立（精确判定 W2 由 doctor D 段补）。
		select {
		case <-ctx.Done():
			return
		case err := <-waitErr:
			r.handleEarlyExit(ctx, t0, err, set, &attempt)
			if ctx.Err() != nil {
				return
			}
			if r.failed() {
				return
			}
		case <-time.After(3 * time.Second):
			set(Connected, "", "")
			err := <-waitErr
			if ctx.Err() != nil {
				return
			}
			_ = err
		}

		if ferr == nil {
			_ = f.Close()
		}

		attempt++
		if attempt > 10 {
			set(Failed, "连续重启 10 次未恢复，最后报错: "+r.tailLog(3),
				"跑 tp doctor 按提示排查")
			return
		}
		d := backoffs[min(attempt-1, len(backoffs)-1)]
		d = jitter(d)
		set(Reconnecting, fmt.Sprintf("第 %d 次重连", attempt), "")
		select {
		case <-ctx.Done():
			return
		case <-time.After(d):
		}
	}
}

// handleEarlyExit 处理秒退：区分"被杀软拦截"与"配置/端口错误"（2026-11-07 实测：腾讯管家会静默删 rathole）。
func (r *rathole) handleEarlyExit(ctx context.Context, t0 time.Time, waitErr error, set func(State, string, string), attempt *int) {
	_ = waitErr
	if ctx.Err() != nil {
		return
	}
	died := time.Since(t0)
	tail := r.tailLog(3)
	if _, err := os.Stat(r.cfg.RatholeBin); err != nil && r.cfg.RatholeBin != "rathole" {
		set(Failed, "找不到引擎文件 "+r.cfg.RatholeBin, "重新安装，或用 --rathole 指定路径")
		return
	}
	if died < 1500*time.Millisecond && tail == "" {
		// 日志里什么都没有就消失 = 大概率被安全软件静默查杀。
		set(Failed, "引擎启动后立刻消失且无日志",
			"你的安全软件（如腾讯管家 / 360 / Defender）可能拦截了 rathole（TP-6001）。把 tp 和 rathole 加入白名单后重试")
		*attempt = 10 // 不再无意义重试
		return
	}
	set(Reconnecting, fmt.Sprintf("引擎退出（%.1fs），日志: %s", died.Seconds(), tail), "")
}

func (r *rathole) failed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status.State == Failed
}

func since(t time.Time) time.Duration {
	if t.IsZero() {
		return 0
	}
	return time.Since(t).Truncate(time.Second)
}

// jitter ±20%（FR-4.3）。
func jitter(d time.Duration) time.Duration {
	f := 0.8 + 0.4*rand.Float64()
	return time.Duration(float64(d) * f)
}

func (r *rathole) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.status
	s.Uptime = since(r.started)
	return s
}

// Stop 停进程、清临时目录（FR-2.1）。
func (r *rathole) Stop() error {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return nil
	}
	r.stopped = true
	cmd, cancel := r.cmd, r.cancel
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	if r.tmpDir != "" {
		_ = os.RemoveAll(r.tmpDir)
	}
	return nil
}

// renderConfig 生成 rathole TOML（D-8：默认 TLS；schema 按 v0.5.0 tag 源码，见 D-1）。
// 注意：v0.5.0 的 schema 是 services 与 transport 嵌套在 [client]/[server] 之内（与主分支文档不同）。
func renderConfig(c *Config) string {
	s := ""
	transport := c.Transport
	if transport == "" {
		transport = "tls"
	}
	if c.Role == "serve" {
		s += fmt.Sprintf("[server]\nbind_addr = \"%s:%d\"\ndefault_token = \"%s\"\n\n", c.Bind, c.ControlPort, c.Token)
		if transport == "tls" {
			s += fmt.Sprintf("[server.transport]\ntype = \"tls\"\n\n[server.transport.tls]\npkcs12 = \"%s\"\npkcs12_password = \"%s\"\n\n", toSlash(c.CertP12), certs.P12Password)
		}
		for _, svc := range c.Services {
			s += fmt.Sprintf("[server.services.%s]\ntype = \"tcp\"\nbind_addr = \"0.0.0.0:%d\"\n\n", svc.Name, svc.PublicPort)
		}
	} else {
		s += fmt.Sprintf("[client]\nremote_addr = \"%s:%d\"\ndefault_token = \"%s\"\n\n", c.Remote, c.RemotePort, c.Token)
		if transport == "tls" {
			s += fmt.Sprintf("[client.transport]\ntype = \"tls\"\n\n[client.transport.tls]\nhostname = \"%s\"\ntrusted_root = \"%s\"\n\n", certs.TLSHostname, toSlash(c.CertRoot))
		}
		for _, svc := range c.Services {
			s += fmt.Sprintf("[client.services.%s]\ntype = \"tcp\"\nlocal_addr = \"%s\"\n\n", svc.Name, svc.LocalAddr)
		}
	}
	return s
}

// toSlash TOML 路径统一正斜杠（Windows 下 Rust fs 也接受）。
func toSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
