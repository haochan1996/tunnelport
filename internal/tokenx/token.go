// Package tokenx 处理配对 token 的生成与解析（FR-3）。
package tokenx

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Scheme 二维码 / 配对 URL 的协议头（FR-3.2）。
const Scheme = "tunnelport"

// New 生成 32 字节随机 token，Base32 无填充展示（52 字符，FR-3.1）。
func New() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

// Valid 检查 token 形态（52 位 Base32 大写）。
func Valid(token string) bool {
	if len(token) != 52 {
		return false
	}
	for _, r := range token {
		if !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZ234567", r) {
			return false
		}
	}
	return true
}

// Pair 结构化配对信息。
type Pair struct {
	Token   string
	Host    string
	Control int    // 控制通道端口（connect 拨号用）
	Public  int    // 对外服务端口（手机连这个，展示用）
	FP      string // 证书 SHA-256 指纹（防中间人，可空）
}

// PairURL 生成配对 URL：tunnelport://REDACTED
func PairURL(host string, control int, token, fp string, public int) string {
	q := fmt.Sprintf("?public=%d", public)
	if fp != "" {
		q += "&fp=" + fp
	}
	return fmt.Sprintf("%s://%s:%d%s#%s", Scheme, host, control, q, token)
}

// Parse 接受两种输入（FR-3.3）：
//  1. 完整 URL：tunnelport://REDACTED
//  2. 纯 token（host / 端口由 --host / --port 提供）
func Parse(s string) (Pair, error) {
	s = strings.TrimSpace(s)
	var p Pair
	if strings.HasPrefix(s, Scheme+"://") {
		u, perr := url.Parse(s)
		if perr != nil {
			return p, fmt.Errorf("配对码格式不对，请重新复制")
		}
		p.Token = u.Fragment
		p.Host = u.Hostname()
		p.Control, _ = strconv.Atoi(u.Port())
		p.Public, _ = strconv.Atoi(u.Query().Get("public"))
		p.FP = u.Query().Get("fp")
		if p.Token == "" || p.Host == "" || p.Control == 0 {
			miss := ""
			if p.Host == "" || p.Control == 0 {
				miss = "地址"
			}
			if p.Token == "" {
				if miss != "" {
					miss += "和"
				}
				miss += "#token 段（链接结尾的 #52位大写字母数字）"
			}
			return p, fmt.Errorf("配对码不完整：缺 %s。请点「从剪贴板读取」或完整复制 tunnelport:// 开头的整条链接", miss)
		}
	} else {
		p.Token = strings.ToUpper(s)
	}
	if !Valid(p.Token) {
		return p, fmt.Errorf("token 不匹配，重新复制（应为 52 位大写字母和数字）")
	}
	return p, nil
}
