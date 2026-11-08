// token 持久化：AES-GCM 加密落盘（FR-3.4：不落明文）。
//
// 用途：tp serve 重启后复用同一 token —— 配对码不会因重启作废（鲁棒性设计）。
// 密钥由机器标识派生（防的是"文件被拷走即明文泄露"，不是防本机 root）；
// 需要重置时用 `tp serve --regenerate`。
package config

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"os/user"
	"path/filepath"
)

// tokenKey 机器标识派生密钥：hostname + username 的哈希（MVP 级别，够用即可）。
func tokenKey() []byte {
	hostname, _ := os.Hostname()
	username := ""
	if u, err := user.Current(); err == nil {
		username = u.Username
	}
	sum := sha256.Sum256([]byte("tunnelport-token-v1|" + hostname + "|" + username))
	return sum[:]
}

// SaveToken 加密保存 token（token.enc，0600）。
func SaveToken(token string) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(tokenKey())
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(token), nil)
	return os.WriteFile(filepath.Join(dir, "token.enc"), []byte(base64.StdEncoding.EncodeToString(sealed)), 0o600)
}

// LoadToken 读取保存的 token；不存在返回 ("", nil)。
func LoadToken() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(dir, "token.enc"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(b)))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(tokenKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("token.enc 损坏")
	}
	nonce, sealed := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", errors.New("token.enc 无法解密（换机器/换用户？）——用 tp serve --regenerate 重新生成")
	}
	return string(plain), nil
}
