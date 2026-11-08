// Package config 管理 ~/.tunnelport 下的本地状态。
// 状态文件只存指纹，不存明文 token（FR-3.4）。
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"time"
)

// State 是 tp 的本地持久化状态（status / doctor 都从这里读）。
type State struct {
	Role      string `json:"role"`     // serve | connect
	Entry     string `json:"entry"`    // 最终入口 host:port（手机填这个）
	Control   string `json:"control"`  // rathole 控制通道 host:port
	TokenFP   string `json:"token_fp"` // token 指纹，仅用于比对提示
	PID       int    `json:"pid"`
	Version   string `json:"version"`
	UpdatedAt string `json:"updated_at"`
}

// Dir 返回状态目录（~/.tunnelport），不存在则创建。
// 鲁棒性：systemd/容器/cron 环境常无 $HOME —— 回退 passwd 主目录，再回退 /var/lib/tunnelport。
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		if u, err2 := user.Current(); err2 == nil && u.HomeDir != "" {
			home = u.HomeDir
		} else {
			home = "/var/lib/tunnelport"
		}
	}
	d := filepath.Join(home, ".tunnelport")
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	return d, nil
}

func StatePath() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "state.json"), nil
}

// SaveState 原子写入状态文件（tmp + rename）。
func SaveState(s *State) error {
	s.UpdatedAt = time.Now().Format(time.RFC3339)
	p, err := StatePath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// LoadState 读状态；不存在返回 nil, nil。
func LoadState() (*State, error) {
	p, err := StatePath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Fingerprint 返回 token 的短指纹（前 12 个 hex 字符，3 组 4 位）。
// 仅用于人眼比对（"这是不是同一个 token"），不参与鉴权（D-2）。
func Fingerprint(token string) string {
	h := sha256.Sum256([]byte(token))
	x := hex.EncodeToString(h[:])[:12]
	return x[0:4] + "-" + x[4:8] + "-" + x[8:12]
}
