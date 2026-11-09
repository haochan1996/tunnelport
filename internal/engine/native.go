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
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	Slot     *int     `json:"slot"` // 偏好端口槽（粘性重连）；nil=无偏好
	Device   string   `json:"d"`    // 设备身份（同设备识别，端口恒定）
	Name     string   `json:"n"`    // 电脑名称（设备清单展示）
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

// slotMan 服务端端口槽分配器 + 会话登记簿（对账回收 / 粘性接管）。
type slotMan struct {
	mu       sync.Mutex
	used     map[int]bool
	max      int // 0 = 不限
	sessions map[int]*slotSession
	bans     map[string]string // device -> name（被移除设备，拒绝重连）
	banPath  string            // 封禁持久化文件
}

// slotSession 一个活跃会话的全部持有物；shutdown 幂等回收。
type slotSession struct {
	slot      int
	device    string
	name      string
	orca      int
	health    int
	since     time.Time
	sess      *smux.Session
	conn      net.Conn
	listeners []net.Listener
	once      sync.Once
}

// DeviceInfo 服务端一个已连接设备（供客户端展示）。
type DeviceInfo struct {
	Name   string `json:"name"`
	Device string `json:"device"`
	Slot   int    `json:"slot"`
	Orca   int    `json:"orca"`
	Health int    `json:"health"`
	Since  string `json:"since"`
	Self   bool   `json:"self"`
	Banned bool   `json:"banned"`
}

func (s *slotSession) shutdown() {
	s.once.Do(func() {
		for _, l := range s.listeners {
			_ = l.Close()
		}
		if s.sess != nil {
			_ = s.sess.Close()
		}
		if s.conn != nil {
			_ = s.conn.Close()
		}
	})
}

// allocPrefer 分配端口槽（设备级端口恒定）：
//  1. 偏好槽可用（空闲/已死）→ 直接给
//  2. 偏好槽被本设备的旧会话占着 → 强接管（重连快于清理也不漂移）
//  3. 否则顺延最小空闲槽
func (m *slotMan) allocPrefer(pref *int, device string) int {
	m.mu.Lock()
	if m.used == nil {
		m.used = map[int]bool{}
		m.sessions = map[int]*slotSession{}
	}
	var zombies []*slotSession
	take := func(k int) bool {
		if m.used[k] {
			s := m.sessions[k]
			if s != nil && s.sess != nil && !s.sess.IsClosed() {
				// 活跃会话：仅同设备可强接管（不同设备绝不抢）
				if device == "" || s.device != device {
					return false
				}
			}
			if s != nil {
				zombies = append(zombies, s) // 出锁后回收（含被接管的同设备旧会话）
				delete(m.sessions, k)
			}
			m.used[k] = false
		}
		m.used[k] = true
		return true
	}
	chosen := -1
	if pref != nil && *pref >= 0 && (m.max == 0 || *pref < m.max) && take(*pref) {
		chosen = *pref
	} else {
		for k := 0; m.max == 0 || k < m.max; k++ {
			if !m.used[k] {
				m.used[k] = true
				chosen = k
				break
			}
		}
	}
	m.mu.Unlock()
	for _, z := range zombies {
		nlog("serve: 会话接管/回收（槽 #%d）", z.slot)
		z.shutdown()
	}
	return chosen
}

// release 租约释放：仅当槽仍归本会话所有才清位（被接管后不再误放）。
func (m *slotMan) release(ss *slotSession) {
	m.mu.Lock()
	if m.sessions[ss.slot] == ss {
		delete(m.sessions, ss.slot)
		m.used[ss.slot] = false
	}
	m.mu.Unlock()
}

func (m *slotMan) attach(slot int, s *slotSession) {
	m.mu.Lock()
	m.sessions[slot] = s
	m.mu.Unlock()
}

func (m *slotMan) free(k int) {
	m.mu.Lock()
	m.used[k] = false
	delete(m.sessions, k)
	m.mu.Unlock()
}

// loadBans 启动时载入封禁簿。
func (m *slotMan) loadBans(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.banPath = path
	m.bans = map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(b, &m.bans)
}

func (m *slotMan) saveBans() {
	if m.banPath == "" {
		return
	}
	b, _ := json.Marshal(m.bans)
	_ = os.WriteFile(m.banPath, b, 0o600)
}

// isBanned 设备是否被移除（拒绝重连）。
func (m *slotMan) isBanned(device string) bool {
	if device == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bans[device] != ""
}

// ban 封禁设备（移除后拒绝重连）。
func (m *slotMan) ban(device, name string) {
	if device == "" {
		return
	}
	m.mu.Lock()
	m.bans[device] = name
	m.saveBans()
	m.mu.Unlock()
	nlog("serve: 设备 %s（%s）已移除并封禁", name, device[:min(8, len(device))])
}

// unban 解封（设备清单"恢复"）。
func (m *slotMan) unban(device string) {
	m.mu.Lock()
	delete(m.bans, device)
	m.saveBans()
	m.mu.Unlock()
}

// snapshot 当前会话清单（设备查询，含已封禁）。
func (m *slotMan) snapshot(self string) []DeviceInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]DeviceInfo, 0, len(m.sessions))
	seen := map[string]bool{}
	for _, s := range m.sessions {
		if s.device != "" {
			seen[s.device] = true
		}
		out = append(out, DeviceInfo{
			Name:   s.name,
			Device: s.device,
			Slot:   s.slot,
			Orca:   s.orca,
			Health: s.health,
			Since:  s.since.Format(time.RFC3339),
			Self:   self != "" && s.device == self,
		})
	}
	for dev, nm := range m.bans {
		if _, online := seen[dev]; !online {
			out = append(out, DeviceInfo{Name: nm, Device: dev, Banned: true})
		}
	}
	return out
}

// kick 踢下线指定槽位会话（断开并清理）。
func (m *slotMan) kick(slot int) bool {
	m.mu.Lock()
	s, ok := m.sessions[slot]
	if ok {
		delete(m.sessions, slot)
		m.used[slot] = false
	}
	m.mu.Unlock()
	if ok {
		nlog("serve: 移除设备（槽 #%d）", slot)
		if s.device != "" {
			m.ban(s.device, s.name) // 有身份的设备：移除后禁止重连
		}
		s.shutdown()
	}
	return ok
}

// reap 对账回收：清掉 smux 已关闭（控制连接已死）的僵尸会话。
func (m *slotMan) reap() {
	m.mu.Lock()
	var dead []*slotSession
	for k, s := range m.sessions {
		if s.sess != nil && s.sess.IsClosed() {
			dead = append(dead, s)
			delete(m.sessions, k)
			m.used[k] = false
		}
	}
	m.mu.Unlock()
	for _, s := range dead {
		nlog("serve: 回收僵尸会话（槽 #%d），端口释放", s.slot)
		s.shutdown()
	}
}

type nativeEngine struct {
	mu      sync.Mutex
	status  Status
	cancel  context.CancelFunc
	started time.Time
	stopped bool
	ports   map[string]int // 服务端分配的实际公开端口
	hint    map[string]int // URL 参考值（兼容旧服务端）
	sess    *smux.Session  // 活跃控制会话（设备查询用）
}

// portHint 返回 URL 参考端口。
func (n *nativeEngine) portHint(name string) (int, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	p, ok := n.hint[name]
	return p, ok
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
	s.Ports = n.ports
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

	// 端口槽基准：orca 的 PublicPort 是设备 0 的 orca 口（三连 base+1），
	// 每设备占 2 口（orca/health）。设备 k：orca=base+2k，health=+1。
	pubBase := 0
	for _, s := range cfg.Services {
		if s.Name == ServiceOrca {
			pubBase = s.PublicPort
		}
	}
	sm := &slotMan{max: cfg.Devices}
	sm.loadBans(filepath.Join(filepath.Dir(cfg.CertPem), "bans.json"))
	// 对账回收：僵尸会话（控制连接已死）定期清理，端口不外泄
	go func() {
		for range time.Tick(10 * time.Second) {
			sm.reap()
		}
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go n.serveConn(ctx, conn, cfg, sm, pubBase)
	}
}

func (n *nativeEngine) serveConn(ctx context.Context, conn net.Conn, cfg Config, sm *slotMan, pubBase int) {
	defer conn.Close()
	nlog("serve: 入站连接 %s，TLS 握手中", conn.RemoteAddr())
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
	if sm.isBanned(hs.Device) {
		_ = writeFrame(conn, []byte(`{"err":"设备已移除。点「更换配对码」重新接入（配对码不用换）"}`))
		return
	}
	nlog("serve: 客户端 %s 已鉴权，注册服务 %v", conn.RemoteAddr(), hs.Services)
	_ = conn.SetDeadline(time.Time{})

	// 分配端口槽并把实际入口端口告知客户端（多设备核心）；带粘性偏好
	slot := sm.allocPrefer(hs.Slot, hs.Device)
	if slot < 0 {
		_ = writeFrame(conn, []byte(`{"err":"设备数已满"}`))
		nlog("serve: 设备槽已满，拒绝 %s", conn.RemoteAddr())
		return
	}
	// 租约释放：attach 前早退用 free，attach 后用 release（被接管时不误放）
	var ss *slotSession
	defer func() {
		if ss == nil {
			sm.free(slot)
		}
	}()
	assigned := map[string]int{
		ServiceOrca:   pubBase + 2*slot,
		ServiceHealth: pubBase + 2*slot + 1,
		"slot":        slot, // 回传槽号：客户端记住，下次重连回原槽
	}
	resp, _ := json.Marshal(assigned)
	if err := writeFrame(conn, resp); err != nil {
		return
	}
	nlog("serve: 分配端口槽 #%d → %v", slot, assigned)

	// 控制连接 TCP keepalive：死对端分钟级兜底检测
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(15 * time.Second)
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
		p, ok := assigned[name]
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

	ss = &slotSession{slot: slot, device: hs.Device, name: hs.Name, orca: assigned[ServiceOrca],
		health: assigned[ServiceHealth], since: time.Now(), sess: sess, conn: conn}
	defer sm.release(ss)
	for _, l := range listeners {
		ss.listeners = append(ss.listeners, l)
	}
	sm.attach(slot, ss)

	// 客户端查询流：@devices → 服务端设备清单（v1.0.21）
	go func() {
		for {
			stm, err := sess.AcceptStream()
			if err != nil {
				return
			}
			go func(stm *smux.Stream) {
				defer stm.Close()
				_ = stm.SetDeadline(time.Now().Add(5 * time.Second))
				req, err := readFrame(stm)
				if err != nil {
					return
				}
				body := string(req)
				switch {
				case body == "@devices":
					b, _ := json.Marshal(sm.snapshot(hs.Device))
					_ = writeFrame(stm, b)
				case strings.HasPrefix(body, "@kick:"):
					slot, _ := strconv.Atoi(strings.TrimPrefix(body, "@kick:"))
					if sm.kick(slot) {
						_ = writeFrame(stm, []byte("OK"))
					} else {
						_ = writeFrame(stm, []byte("ERR"))
					}
				case strings.HasPrefix(body, "@unban:"):
					sm.unban(strings.TrimPrefix(body, "@unban:"))
					_ = writeFrame(stm, []byte("OK"))
				}
			}(stm)
		}
	}()

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
	// 统一幂等收尾（shutdown 关监听/会话/连接），再等 accept goroutine 退出
	ss.shutdown()
	wg.Wait()
	return
}

// runClient：connect 侧 —— 拨出、鉴权、按流拨本地。
func (n *nativeEngine) runClient(ctx context.Context, cfg Config) error {
	// URL 参考端口（服务端为旧版本时回退显示用）
	n.mu.Lock()
	if n.hint == nil {
		n.hint = map[string]int{}
	}
	for _, s := range cfg.Services {
		if s.PublicPort > 0 {
			n.hint[s.Name] = s.PublicPort
		}
	}
	n.mu.Unlock()
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
	nlog("client: 已连接 %s:%d，TLS 握手中", cfg.Remote, cfg.RemotePort)
	defer conn.Close()

	services := map[string]string{}
	var names []string
	for _, s := range cfg.Services {
		services[s.Name] = s.LocalAddr
		names = append(names, s.Name)
	}
	var slotPref *int
	if v, ok := readSlotHint(cfg); ok {
		slotPref = &v
	}
	hostName, _ := os.Hostname() // computer name for device list
	hs := nativeHandshake{Token: cfg.Token, Version: nativeProtoVersion, Services: names,
		Slot: slotPref, Device: deviceID(cfg), Name: hostName}
	b, _ := json.Marshal(hs)
	if err := writeFrame(conn, b); err != nil {
		return err
	}
	resp, err := readFrame(conn)
	if err != nil {
		return err
	}
	if strings.HasPrefix(string(resp), "{") && strings.Contains(string(resp), "err") {
		nlog("client: 被拒（%s）", resp)
		if strings.Contains(string(resp), "已移除") {
			resetDeviceID(cfg) // 作废设备身份：重新配对即以新身份回归
		}
		return fmt.Errorf("服务端拒绝：%s", resp)
	}
	assigned := map[string]int{}
	if strings.HasPrefix(string(resp), "{") {
		_ = json.Unmarshal(resp, &assigned)
	}
	if len(assigned) == 0 {
		// 兼容旧响应 OK：退回 URL 里的端口
		if p, ok := n.portHint(ServiceOrca); ok {
			assigned[ServiceOrca] = p
		}
	}
	n.mu.Lock()
	n.ports = assigned
	n.mu.Unlock()
	nlog("client: 已分配端口 %v", assigned)
	if k, ok := assigned["slot"]; ok {
		saveSlotHint(cfg, k) // 记住槽号：重连回原入口端口
	}
	nlog("client: 鉴权通过，服务 %v", names)

	sess, err := smux.Client(conn, muxCfg())
	if err != nil {
		return err
	}
	n.mu.Lock()
	n.sess = sess
	n.mu.Unlock()
	defer func() {
		n.mu.Lock()
		if n.sess == sess {
			n.sess = nil
		}
		n.mu.Unlock()
	}()
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

// ---------- 槽位粘性：设备身份 + 槽位记忆 ----------

// readSlotHint 读取上次分配的槽号。
func readSlotHint(cfg Config) (int, bool) {
	if cfg.CertRoot == "" {
		return 0, false
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(cfg.CertRoot), "slot"))
	if err != nil {
		return 0, false
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || v < 0 {
		return 0, false
	}
	return v, true
}

// saveSlotHint 落盘槽号。
func saveSlotHint(cfg Config, k int) {
	if cfg.CertRoot == "" {
		return
	}
	_ = os.WriteFile(filepath.Join(filepath.Dir(cfg.CertRoot), "slot"), []byte(strconv.Itoa(k)), 0o600)
}

// resetDeviceID 作废设备身份（被移除后重新配对用）。
func resetDeviceID(cfg Config) {
	if cfg.CertRoot == "" {
		return
	}
	_ = os.Remove(filepath.Join(filepath.Dir(cfg.CertRoot), "device"))
	nlog("client: 设备身份已作废，请重新配对")
}

// deviceID 本机稳定设备身份（首次生成后持久复用）。
// 同一配置目录 = 同一设备；服务端凭它实现"一设备一固定端口"。
func deviceID(cfg Config) string {
	if cfg.CertRoot == "" {
		return ""
	}
	p := filepath.Join(filepath.Dir(cfg.CertRoot), "device")
	if b, err := os.ReadFile(p); err == nil {
		if id := strings.TrimSpace(string(b)); id != "" {
			return id
		}
	}
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	id := hex.EncodeToString(buf[:])
	_ = os.WriteFile(p, []byte(id), 0o600)
	return id
}

// Devices 查询服务端已连接设备（经控制连接 @devices 流）。
func (n *nativeEngine) Devices() ([]DeviceInfo, error) {
	n.mu.Lock()
	sess := n.sess
	n.mu.Unlock()
	if sess == nil {
		return nil, fmt.Errorf("未连接")
	}
	stm, err := sess.OpenStream()
	if err != nil {
		return nil, err
	}
	defer stm.Close()
	_ = stm.SetDeadline(time.Now().Add(5 * time.Second))
	if err := writeFrame(stm, []byte("@devices")); err != nil {
		return nil, err
	}
	resp, err := readFrame(stm)
	if err != nil {
		return nil, err
	}
	var list []DeviceInfo
	if err := json.Unmarshal(resp, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// Kick 踢下线服务端指定槽位的设备（设备清单"移除"）。
func (n *nativeEngine) Kick(slot int) error {
	n.mu.Lock()
	sess := n.sess
	n.mu.Unlock()
	if sess == nil {
		return fmt.Errorf("未连接")
	}
	stm, err := sess.OpenStream()
	if err != nil {
		return err
	}
	defer stm.Close()
	_ = stm.SetDeadline(time.Now().Add(5 * time.Second))
	if err := writeFrame(stm, []byte("@kick:"+strconv.Itoa(slot))); err != nil {
		return err
	}
	resp, err := readFrame(stm)
	if err != nil {
		return err
	}
	if string(resp) != "OK" {
		return fmt.Errorf("移除失败（设备可能已离线）")
	}
	return nil
}

// Unban 解封被移除的设备（设备清单"恢复"）。
func (n *nativeEngine) Unban(device string) error {
	n.mu.Lock()
	sess := n.sess
	n.mu.Unlock()
	if sess == nil {
		return fmt.Errorf("未连接")
	}
	stm, err := sess.OpenStream()
	if err != nil {
		return err
	}
	defer stm.Close()
	_ = stm.SetDeadline(time.Now().Add(5 * time.Second))
	if err := writeFrame(stm, []byte("@unban:"+device)); err != nil {
		return err
	}
	_, err = readFrame(stm)
	return err
}
