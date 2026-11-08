package engine

import (
	"strings"
	"testing"
)

func TestRenderConfigTLSBothRoles(t *testing.T) {
	serve := renderConfig(&Config{
		Role: "serve", Bind: "0.0.0.0", ControlPort: 30123, Token: "T",
		Transport: "tls", CertP12: "C:/x/identity.p12",
		Services: []Service{
			{Name: "orca", PublicPort: 6768},
			{Name: "tp-health", PublicPort: 6769},
		},
	})
	for _, want := range []string{
		`[server]`, `bind_addr = "0.0.0.0:30123"`,
		`[server.transport]`, `type = "tls"`,
		`[server.transport.tls]`, `pkcs12 = "C:/x/identity.p12"`, // 正斜杠
		`[server.services.orca]`, `bind_addr = "0.0.0.0:6768"`,
		`[server.services.tp-health]`,
	} {
		if !strings.Contains(serve, want) {
			t.Fatalf("serve 配置缺少 %q:\n%s", want, serve)
		}
	}

	conn := renderConfig(&Config{
		Role: "connect", Remote: "1.2.3.4", RemotePort: 30123, Token: "T",
		Transport: "tls", CertRoot: "/root.pem",
		Services: []Service{{Name: "orca", LocalAddr: "127.0.0.1:6768"}},
	})
	for _, want := range []string{
		`[client]`, `remote_addr = "1.2.3.4:30123"`,
		`[client.transport]`, `hostname = "tunnelport"`, `trusted_root = "/root.pem"`,
		`[client.services.orca]`, `local_addr = "127.0.0.1:6768"`,
	} {
		if !strings.Contains(conn, want) {
			t.Fatalf("connect 配置缺少 %q:\n%s", want, conn)
		}
	}
}

func TestRenderConfigTCPFallback(t *testing.T) {
	s := renderConfig(&Config{Role: "serve", Bind: "0.0.0.0", ControlPort: 1, Token: "T", Transport: "tcp"})
	if strings.Contains(s, "transport") {
		t.Fatalf("tcp 回退不应包含 transport 段:\n%s", s)
	}
	if strings.Contains(s, "pkcs12") {
		t.Fatal("tcp 回退不应包含 pkcs12")
	}
}

func TestRenderConfigDefaultTransportIsTLS(t *testing.T) {
	// v1.1 起默认加密（D-8）：Transport 留空必须是 tls
	s := renderConfig(&Config{Role: "serve", Bind: "0.0.0.0", ControlPort: 1, Token: "T", CertP12: "/a.p12"})
	if !strings.Contains(s, `type = "tls"`) {
		t.Fatal("默认 transport 必须是 tls")
	}
}

func TestRenderConfigSchemaNesting(t *testing.T) {
	// v0.5.0 schema：transport/services 嵌套在 [server]/[client] 内（D-1 教训）
	s := renderConfig(&Config{Role: "serve", Bind: "0.0.0.0", ControlPort: 1, Token: "T", Transport: "tls", CertP12: "/a"})
	if !strings.Contains(s, "[server.transport]") {
		t.Fatal("必须使用嵌套 [server.transport]（v0.5.0 schema）")
	}
	if strings.Contains(s, "\n[transport]") {
		t.Fatal("不得出现平铺的 [transport]（那是主分支格式）")
	}
}
