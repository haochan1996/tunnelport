// Package address 生成入口地址、配对 URL 与终端二维码（FR-6）。
package address

import (
	"bytes"
	"fmt"

	"github.com/mdp/qrterminal/v3"
)

// PairURL 配对 URL（FR-3.2）。fp 为证书 SHA-256 指纹（防中间人），可为空。
func PairURL(host string, port int, token, fp string) string {
	q := ""
	if fp != "" {
		q = "?fp=" + fp
	}
	return fmt.Sprintf("tunnelport://REDACTED host, port, q, token)
}

// EntryURL 手机 Orca 填写的地址（host:port）。
func EntryURL(host string, port int) string {
	return fmt.Sprintf("%s:%d", host, port)
}

// HealthURL 外部可达性自检地址（FR-7.3），手机用 4G 打开。
func HealthURL(host string, port int) string {
	return fmt.Sprintf("http://%s:%d/health", host, port)
}

// RenderQR 终端 ASCII 二维码（FR-6.2）。
func RenderQR(s string) string {
	var buf bytes.Buffer
	qrterminal.GenerateWithConfig(s, qrterminal.Config{
		Level:      qrterminal.L,
		Writer:     &buf,
		HalfBlocks: true,
		QuietZone:  1,
	})
	return buf.String()
}
