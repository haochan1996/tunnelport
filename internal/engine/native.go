// 自研隧道引擎（Go 原生）—— 协议 v1，加密：crypto/tls。
//
// 为什么自研（D-14）：rathole 等知名隧道引擎被杀软按专杀特征拦截
// （HackTool/Rathole.a，落盘即杀，无法规避）；自研引擎不在任何特征库中，
// tp 与引擎均为干净二进制，分发与运行不再受杀软影响。
//
// 协议（窄而正确，只做需要的能力）：
//
//	控制连接 = TLS + token 握手（首帧 JSON）+ smux 流多路复用（15s keepalive）
//	被转发的每条连接 = 一条 smux 流，流首帧为服务名（4 字节长度前缀）
//	serve 在注册后绑定各服务公开端口；入站连接开流送服务名+数据，connect 侧拨本地
//
// v2 路径：握手层可替换为 noise（token 作 PSK，D-8 的目标）。
package engine

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/xtaci/smux/v2"

	"github.com/haochan1996/tunnelport/internal/certs"
)

const nativeProtoVersion = "1"

var errNativeAuth = errors.New("token 不匹配")

type nativeHandshake struct {
	Token    string   `json:"t"`
	Version  string   `json:"v"`
	Services []string `json:"s"`
}

// ---------- 帧 ----------

func writeFrame(w io.Writer, payload []byte) error {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func readFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > 1<<20 {
		return nil, fmt.Errorf("帧过长: %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func writeService(w io.Writer, name string) error { return writeFrame(w, []byte(name)) }

func readService(r io.Reader) (string, error) {
	b, err := readFrame(r)
	return string(b), err
}

func tokenEq(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func muxCfg() *smux.Config {
	c := smux.DefaultConfig()
	c.KeepAliveInterval = 15 * time.Second
	return c
}

// pipe 双向并发转发；任一端结束即关闭两端。
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(a, b); done <- struct{}{} }()
	go func() { _, _ = io.Copy(b, a); done <- struct{}{} }()
	<-done
	_ = a.Close()
	_ = b.Close()
}

// ---------- 引擎实现 ----------

type nativeEngine struct {
	mu      sync.Mutex
	status  Status
	cancel  context.CancelFunc
	started time.Time
	stopped bool
}

// NewNative 返回自研引擎（默认引擎，D-14）。
func NewNative() Engine { return &nativeEngine{} }

func (n *nativeEngine) Start(ctx context.Context, cfg Config) error {
	if cfg.Transport == "" {
		cfg.Transport = "tls"
	}
	runCtx, cancel := context.WithCancel(ctx)
	n.cancel = cancel
	n.started = time.Now()
	n.set(Connecting, "", "")
	go n.loop(runCtx, cfg)
	return nil
}

func (n *nativeEngine) set(s State, reason, hint string) {
	n.mu.Lock()
	n.status = Status{State: s, Reason: reason, Hint: hint, Uptime: since(n.started)}
	n.mu.Unlock()
}

func (n *nativeEngine) Status() Status {
	n.mu.Lock()
	defer n.mu.Unlock()
	s := n.status
	s.Uptime = since(n.started)
	return s
}

func (n *nativeEngine) Stop() error {
	n.mu.Lock()
	if n.stopped {
		n.mu.Unlock()
		return nil
	}
	n.stopped = true
	cancel := n.cancel
	n.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// loop：会话生命周期 + 1s→30s 退避重连（与 rathole 包装层同策略）。
func (n *nativeEngine) loop(ctx context.Context, cfg Config) {
	backoffs := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second}
	attempt := 0
	for {
		if ctx.Err() != nil {
			return
		}
		var err error
		if cfg.Role == "serve" {
			err = n.runServer(ctx, cfg)
		} else {
			err = n.runClient(ctx, cfg)
		}
		if ctx.Err() != nil {
			return
		}
		attempt++
		if attempt > 30 {
			n.set(Failed, fmt.Sprintf("连续重连 %d 次未恢复: %v", attempt, err), "跑 tp doctor 按提示排查")
			return
		}
		d := jitter(backoffs[min(attempt-1, len(backoffs)-1)])
		n.set(Reconnecting, fmt.Sprintf("第 %d 次重连（%v）", attempt, err), "")
		select {
		case <-ctx.Done():
			return
		case <-time.After(d):
		}
	}
}

// runServer：serve 侧 —— 控制监听、鉴权、绑定公开端口、入站转发。
func (n *nativeEngine) runServer(ctx context.Context, cfg Config) error {
	tlsCfg, err := serverTLSConfig(cfg.CertPem, cfg.CertKey)
	if err != nil {
		nlog("serve: 加载证书失败: %v", err)
		n.set(Failed, "加载服务端证书失败: "+err.Error(), "重新运行 tp serve 生成证书")
		return err
	}
	ln, err := tls.Listen("tcp", net.JoinHostPort(cfg.Bind, strconv.Itoa(cfg.ControlPort)), tlsCfg)
	if err != nil {
		nlog("serve: 监听 %s:%d 失败: %v", cfg.Bind, cfg.ControlPort, err)
		return fmt.Errorf("监听 %s:%d 失败: %w", cfg.Bind, cfg.ControlPort, err)
	}
	nlog("serve: 控制通道监听 %s:%d (tls)", cfg.Bind, cfg.ControlPort)
	defer ln.Close()
	go func() { <-ctx.Done(); _ = ln.Close() }()

	ports := map[string]int{}
	for _, s := range cfg.Services {
		ports[s.Name] = s.PublicPort
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go n.serveConn(ctx, conn, cfg, ports)
	}
}

func (n *nativeEngine) serveConn(ctx context.Context, conn net.Conn, cfg Config, ports map[string]int) {
	defer conn.Close()
	nlog("serve: 入站连接 %s，TLS 掉手中", conn.RemoteAddr())
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	frame, err := readFrame(conn)
	if err != nil {
		nlog("serve: %s 握手/首帧失败: %v", conn.RemoteAddr(), err)
		return
	}
	var hs nativeHandshake
	if json.Unmarshal(frame, &hs) != nil || !tokenEq(hs.Token, cfg.Token) || hs.Version != nativeProtoVersion {
		nlog("serve: 鉴权失败来自 %s", conn.RemoteAddr())
		_ = writeFrame(conn, []byte("ERR"))
		return
	}
	nlog("serve: 客户端 %s 已鉴权，注册服务 %v", conn.RemoteAddr(), hs.Services)
	_ = conn.SetDeadline(time.Time{})
	if err := writeFrame(conn, []byte("OK")); err != nil {
		return
	}

	sess, err := smux.Server(conn, muxCfg())
	if err != nil {
		return
	}
	defer sess.Close()

	listeners := map[string]net.Listener{}
	defer func() {
		for _, l := range listeners {
			_ = l.Close()
		}
	}()
	for _, name := range hs.Services {
		p, ok := ports[name]
		if !ok {
			continue
		}
		l, err := net.Listen("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(p)))
		if err != nil {
			nlog("serve: 端口 %d 绑定失败: %v", p, err)
			n.set(Degraded, fmt.Sprintf("端口 %d 绑定失败: %v", p, err), "检查该端口是否被占用")
			continue
		}
		nlog("serve: 服务 %s 监听 0.0.0.0:%d", name, p)
		listeners[name] = l
	}
	n.set(Connected, "", "")

	var wg sync.WaitGroup
	for name, l := range listeners {
		name, l := name, l
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				uc, err := l.Accept()
				if err != nil {
					return
				}
				go func(uc net.Conn) {
					stream, err := sess.OpenStream()
					if err != nil {
						_ = uc.Close()
						return
					}
					if err := writeService(stream, name); err != nil {
						_ = stream.Close()
						_ = uc.Close()
						return
					}
					pipe(uc, stream)
				}(uc)
			}
		}()
	}
	for {
		if sess.IsClosed() || ctx.Err() != nil {
			break
		}
		time.Sleep(time.Second)
	}
	// 关闭监听让 accept goroutine 退出，再等待（defer 顺序易死锁，显式收尾）
	for _, l := range listeners {
		_ = l.Close()
	}
	wg.Wait()
	return
}

// runClient：connect 侧 —— 拨出、鉴权、按流拨本地。
func (n *nativeEngine) runClient(ctx context.Context, cfg Config) error {
	tlsCfg, err := clientTLSConfig(cfg.CertRoot)
	if err != nil {
		return err
	}
	d := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := tls.DialWithDialer(d, "tcp", net.JoinHostPort(cfg.Remote, strconv.Itoa(cfg.RemotePort)), tlsCfg)
	if err != nil {
		nlog("client: 连接 %s:%d 失败: %v", cfg.Remote, cfg.RemotePort, err)
		return fmt.Errorf("连接中继失败: %w", err)
	}
	nlog("client: 已连接 %s:%d，TLS 掉手中", cfg.Remote, cfg.RemotePort)
	defer conn.Close()

	services := map[string]string{}
	var names []string
	for _, s := range cfg.Services {
		services[s.Name] = s.LocalAddr
		names = append(names, s.Name)
	}
	hs := nativeHandshake{Token: cfg.Token, Version: nativeProtoVersion, Services: names}
	b, _ := json.Marshal(hs)
	if err := writeFrame(conn, b); err != nil {
		return err
	}
	resp, err := readFrame(conn)
	if err != nil {
		return err
	}
	if string(resp) != "OK" {
		nlog("client: 鉴权被拒")
		return errNativeAuth
	}
	nlog("client: 鉴权通过，服务 %v", names)

	sess, err := smux.Client(conn, muxCfg())
	if err != nil {
		return err
	}
	defer sess.Close()
	n.set(Connected, "", "")

	for {
		stream, err := sess.AcceptStream()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("控制连接断开: %w", err)
		}
		go func(stream *smux.Stream) {
			name, err := readService(stream)
			if err != nil {
				_ = stream.Close()
				return
			}
			local, ok := services[name]
			if !ok {
				_ = stream.Close()
				return
			}
			up, err := net.DialTimeout("tcp", local, 5*time.Second)
			if err != nil {
				_ = stream.Close()
				return
			}
			pipe(up, stream)
		}(stream)
	}
}

func serverTLSConfig(certPath, keyPath string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, nil
}

func clientTLSConfig(rootPath string) (*tls.Config, error) {
	cfg := &tls.Config{ServerName: certs.TLSHostname, MinVersion: tls.VersionTLS12}
	if rootPath != "" {
		pem, err := os.ReadFile(rootPath)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("根证书解析失败: %s", rootPath)
		}
		cfg.RootCAs = pool
	}
	return cfg, nil
}

// nlog 把引擎事件写入 ~/.tunnelport/engine.log（tp logs 可见；失败不致命）。
func nlog(format string, args ...any) {
	if d, err := os.UserHomeDir(); err == nil {
		if f, err := os.OpenFile(filepath.Join(d, ".tunnelport", "engine.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			fmt.Fprintf(f, time.Now().Format(time.RFC3339)+" "+format+"\n", args...)
			_ = f.Close()
		}
	}
}
