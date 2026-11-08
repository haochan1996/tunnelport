package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// version 由 CI -ldflags 注入（tag 名）。
var version = "dev"

// UpdateDTO 版本检查结果。
type UpdateDTO struct {
	Current    string `json:"current"`
	Latest     string `json:"latest"`
	HasUpdate  bool   `json:"hasUpdate"`
	AssetName  string `json:"assetName"`
	AssetURL   string `json:"assetUrl"`
	ReleaseURL string `json:"releaseUrl"`
}

type ghRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
	HTMLURL    string `json:"html_url"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// CheckUpdate 对比 GitHub Releases 最新版。
func (App) CheckUpdate() (UpdateDTO, error) {
	out := UpdateDTO{Current: version, Latest: version}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/haochan1996/tunnelport/releases?per_page=10")
	if err != nil {
		return out, fmt.Errorf("网络不可用：%w", err)
	}
	defer resp.Body.Close()
	var rels []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rels); err != nil {
		return out, fmt.Errorf("解析失败：%w", err)
	}
	var latest *ghRelease
	for i := range rels {
		if !rels[i].Draft {
			latest = &rels[i]
			break
		}
	}
	if latest == nil {
		return out, fmt.Errorf("暂无发布版本")
	}
	out.Latest, out.ReleaseURL = latest.TagName, latest.HTMLURL
	out.HasUpdate = compareVersions(latest.TagName, version) > 0
	want := assetName()
	for _, a := range latest.Assets {
		if a.Name == want {
			out.AssetName, out.AssetURL = a.Name, a.URL
			break
		}
	}
	return out, nil
}

// DoUpdate 下载最新版并原位替换本程序；返回给用户的提示。
func (App) DoUpdate() (string, error) {
	u, err := (&App{}).CheckUpdate()
	if err != nil {
		return "", err
	}
	if !u.HasUpdate {
		return "已是最新版（" + u.Current + "）", nil
	}
	if u.AssetURL == "" {
		return "请到发布页手动更新：" + u.ReleaseURL, nil
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(u.AssetURL)
	if err != nil {
		return "", fmt.Errorf("下载失败：%w", err)
	}
	defer resp.Body.Close()
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	tmp := exe + ".new"
	f, err := os.Create(tmp)
	if err != nil {
		return "", fmt.Errorf("写入失败：%w", err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return "", fmt.Errorf("下载中断：%w", err)
	}
	f.Close()
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("替换失败（%v）。可手动下载：%s", err, u.ReleaseURL)
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Rename(old, exe) // 回滚
		return "", fmt.Errorf("替换失败（%v）", err)
	}
	_ = os.Remove(old)
	return "已更新到 " + u.Latest + "，重启应用生效", nil
}

// assetName 当前平台对应的发布资产名。
func assetName() string {
	base := "tunnelport"
	if exe, err := os.Executable(); err == nil {
		n := filepath.Base(exe)
		if strings.HasPrefix(n, "tp") && !strings.HasPrefix(n, "tunnelport") {
			base = "tp"
		}
	}
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("%s_%s_%s%s", base, runtime.GOOS, runtime.GOARCH, ext)
}

// 简单版本比较：v0.1.1 > v0.1.0
func compareVersions(a, b string) int {
	pa, pb := verParts(a), verParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] - pb[i]
		}
	}
	return 0
}

func verParts(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	var out [3]int
	for i, p := range strings.Split(v, ".") {
		if i > 2 {
			break
		}
		n, _ := strconv.Atoi(p)
		out[i] = n
	}
	return out
}

// OpenURL 用系统浏览器打开链接。
func (App) OpenURL(u string) error {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case "darwin":
		c = exec.Command("open", u)
	default:
		c = exec.Command("xdg-open", u)
	}
	return c.Start()
}
