// Package certs 管理自签名证书：生成、指纹、分发（D-8 链路加密）。
//
// 设计要点：
//   - 证书 CN/SAN 固定为 "tunnelport"，rathole 客户端以 hostname=tunnelport 校验
//   - serve 端持有 cert+key（导出 PKCS#12 给 rathole）；connect 端只拿证书 PEM
//     作为 trusted_root（指纹经配对 URL 传递，防中间人）
package certs

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"software.sslmate.com/src/go-pkcs12"
)

// TLSHostname rathole 客户端校验用的固定主机名（证书 CN/SAN 与之一致）。
const TLSHostname = "tunnelport"

// P12Password PKCS#12 包密码。文件本身 0600，密码只防误导出。
const P12Password = "tunnelport"

// Ensure 保证 dir 下有一套自签名证书（已有则复用），返回证书 SHA-256 指纹。
// 产物：cert.pem（含公钥证书）、key.pem（0600 私钥）、identity.p12（rathole server 用）。
func Ensure(dir string) (string, error) {
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	p12Path := filepath.Join(dir, "identity.p12")

	if _, err := os.Stat(certPath); err == nil {
		fp, _, err := fingerprintFile(certPath)
		return fp, err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", err
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: TLSHostname, Organization: []string{"tunnelport"}},
		DNSNames:              []string{TLSHostname},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true, // 自签且同时作为客户端的 trusted_root
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return "", err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return "", err
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return "", err
	}
	p12, err := pkcs12.Modern.Encode(key, cert, nil, P12Password) // AES-256-CBC/PBES2：遗留 RC2 会被 OpenSSL 3 拒绝（2026-11-08 实测）
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(p12Path, p12, 0o600); err != nil {
		return "", err
	}
	return Fingerprint(cert), nil
}

// Fingerprint 证书 SHA-256 指纹（hex 64 位，配对 URL 里携带用于防中间人）。
func Fingerprint(cert *x509.Certificate) string {
	h := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(h[:])
}

func fingerprintFile(pemPath string) (string, *x509.Certificate, error) {
	b, err := os.ReadFile(pemPath)
	if err != nil {
		return "", nil, err
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		return "", nil, errors.New("证书 PEM 解析失败")
	}
	cert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return "", nil, err
	}
	return Fingerprint(cert), cert, nil
}

// TOFUProbe 首连探测：抓取对端证书，与配对码携带的指纹比对（如有），
// 通过则把证书落盘为 rathole 客户端的 trusted_root。
// 返回落盘路径。
// TOFUProbe 首连探测：抓取对端证书，与配对码携带的指纹比对（如有），
// 通过则把证书落盘为 rathole 客户端的 trusted_root。
// 返回落盘路径。
func TOFUProbe(host string, port int, expectFp, dir string) (string, error) {
	d := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 5 * time.Second}, // 防防火墙 DROP 时永久挂起（稳定性审查 #1）
		Config:    &tls.Config{InsecureSkipVerify: true}, //nolint:gosec —— 目的就是先取证书再人工比对
	}
	conn, err := d.DialContext(context.Background(), "tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return "", fmt.Errorf("TLS 探测失败: %w（对端旧版本未开 TLS 可 --transport tcp 回退；若配对码里的地址不对（如代理网络），用 --host 覆盖）", err)
	}
	tlsConn := conn.(*tls.Conn)
	state := tlsConn.ConnectionState()
	_ = conn.Close()
	if len(state.PeerCertificates) == 0 {
		return "", errors.New("对端未出示证书")
	}
	cert := state.PeerCertificates[0]
	fp := Fingerprint(cert)
	if expectFp != "" && fp != expectFp {
		return "", fmt.Errorf("证书指纹不匹配（配对码 %s… / 实际 %s…），可能存在中间人，已中止", short(expectFp), short(fp))
	}
	root := filepath.Join(dir, "root.pem")
	if err := os.WriteFile(root, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o644); err != nil {
		return "", err
	}
	return root, nil
}

// EnsureTerminator 端到端终结器用的独立证书（cert+key），目录隔离。
func EnsureTerminator(dir string) (tls.Certificate, error) {
	td := filepath.Join(dir, "term")
	_ = os.MkdirAll(td, 0o700)
	crt := filepath.Join(td, "cert.pem")
	key := filepath.Join(td, "key.pem")
	if _, err := os.Stat(crt); errors.Is(err, os.ErrNotExist) {
		key2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return tls.Certificate{}, err
		}
		serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
		tpl := &x509.Certificate{
			SerialNumber:          serial,
			Subject:               pkix.Name{CommonName: TLSHostname + "-local"},
			DNSNames:              []string{TLSHostname, TLSHostname + "-local"},
			NotBefore:             time.Now().Add(-time.Hour),
			NotAfter:              time.Now().AddDate(10, 0, 0),
			KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			BasicConstraintsValid: true,
		}
		der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key2.PublicKey, key2)
		if err != nil {
			return tls.Certificate{}, err
		}
		keyDER, err := x509.MarshalECPrivateKey(key2)
		if err != nil {
			return tls.Certificate{}, err
		}
		if err := os.WriteFile(crt, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
			return tls.Certificate{}, err
		}
		if err := os.WriteFile(key, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
			return tls.Certificate{}, err
		}
	}
	return tls.LoadX509KeyPair(crt, key)
}

func short(fp string) string {
	if len(fp) > 12 {
		return fp[:12]
	}
	return fp
}
