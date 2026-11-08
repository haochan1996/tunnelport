package detect

import (
	"net"
	"testing"
)

func TestNextFreePortSkipsOccupied(t *testing.T) {
	// 占住 16969，从 16969 顺延应返回 16970
	l, err := net.Listen("tcp", "0.0.0.0:16969")
	if err != nil {
		t.Skipf("无法占位端口: %v", err)
	}
	defer l.Close()

	got := NextFreePort(16969)
	if got != 16970 {
		t.Fatalf("应顺延到 16970，实际 %d", got)
	}
}

func TestNextFreePortAvoid(t *testing.T) {
	// 16980 空闲但被 avoid → 应返回 16981
	got := NextFreePort(16980, 16980)
	if got != 16981 {
		t.Fatalf("avoid 生效应返回 16981，实际 %d", got)
	}
}

func TestNextFreePortZero(t *testing.T) {
	if got := NextFreePort(0); got != 0 {
		t.Fatalf("preferred<=0 应返回 0，实际 %d", got)
	}
}

func TestPickPortPreferred(t *testing.T) {
	// 占住 16990，显式指定 16990 应报错
	l, err := net.Listen("tcp", "0.0.0.0:16990")
	if err != nil {
		t.Skipf("无法占位端口: %v", err)
	}
	defer l.Close()

	if _, err := PickPort(16990); err == nil {
		t.Fatal("占用中的显式端口应报错")
	}
}
