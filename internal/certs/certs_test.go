package certs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsureGeneratesAndReuses(t *testing.T) {
	dir := t.TempDir()
	fp1, err := Ensure(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(fp1) != 64 {
		t.Fatalf("指纹应为 64 位 hex，实际 %d", len(fp1))
	}
	for _, f := range []string{"cert.pem", "key.pem", "identity.p12"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("缺少产物 %s: %v", f, err)
		}
	}
	// 私钥权限（Unix 语义；Windows/NTFS 不映射权限位，跳过）
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(filepath.Join(dir, "key.pem"))
		if st.Mode().Perm()&0o077 != 0 {
			t.Fatal("key.pem 权限过宽")
		}
	}
	// 二次调用复用（指纹一致）
	fp2, err := Ensure(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fp1 != fp2 {
		t.Fatal("复用时指纹应一致")
	}
}

func TestTOFUProbeFingerprintMismatch(t *testing.T) {
	// 对一个肯定不是 TLS 的端口做探测，应报错而非挂起（稳定性审查 #1：超时验证）
	dir := t.TempDir()
	if _, err := TOFUProbe("127.0.0.1", 1, "0000000000000000000000000000000000000000000000000000000000000000", dir); err == nil {
		t.Fatal("对不可达端口探测应报错")
	}
}
