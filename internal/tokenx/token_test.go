package tokenx

import (
	"strings"
	"testing"
)

func TestNewAndValid(t *testing.T) {
	tok, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if !Valid(tok) {
		t.Fatalf("生成的 token 不合法: %s", tok)
	}
	if len(tok) != 52 {
		t.Fatalf("token 长度应为 52，实际 %d", len(tok))
	}
	if Valid(strings.ToLower(tok)) {
		t.Fatal("小写 token 不应通过")
	}
	if Valid(tok[:51]) {
		t.Fatal("51 位 token 不应通过")
	}
}

func TestPairURLAndParse(t *testing.T) {
	tok, _ := New()
	host, control, public, fp := "1.2.3.4", 30123, 6768, "abcdef1234567890"
	url := PairURL(host, control, tok, fp, public)

	p, err := Parse(url)
	if err != nil {
		t.Fatal(err)
	}
	if p.Token != tok || p.Host != host || p.Control != control || p.Public != public || p.FP != fp {
		t.Fatalf("解析字段不匹配: %+v", p)
	}
}

func TestParseBareToken(t *testing.T) {
	tok, _ := New()
	p, err := Parse(tok)
	if err != nil {
		t.Fatal(err)
	}
	if p.Token != tok || p.Control != 0 || p.Public != 0 {
		t.Fatalf("纯 token 解析异常: %+v", p)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []string{
		"",                            // 空
		"SHORT",                       // 非 52 位
		"https://1.2.3.4:30123#x",     // 错误协议
		"tunnelport://1.2.3.4:30123#", // 缺 token
		"tunnelport://:30123#x",       // 缺 host —— 注意 token 也是非法的
	}
	for i, c := range cases {
		if _, err := Parse(c); err == nil {
			t.Fatalf("case %d (%q) 应当报错", i, c)
		}
	}
}

func TestParseURLMissingFragment(t *testing.T) {
	// URL 缺 #token → 报"不完整"
	if _, err := Parse("tunnelport://1.2.3.4:30123?public=6768"); err == nil {
		t.Fatal("缺 fragment 应报错")
	}
}
