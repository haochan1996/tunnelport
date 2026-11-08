package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// latestTag 取最新非 draft 版本。三级策略：
//  1. /releases/latest 重定向（浏览器口径，不受 API 配额限制）
//  2. releases 列表页解析（覆盖 prerelease——"latest" 语义不含预发布）
//  3. GitHub API 回退（匿名配额 60 次/时，共享出口 IP 常耗尽）
func latestTag() (tag, pageURL string, rel *ghRelease, err error) {
	noRedirect := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if resp, e := noRedirect.Get("https://github.com/haochan1996/tunnelport/releases/latest"); e == nil {
		defer resp.Body.Close()
		loc := resp.Header.Get("Location")
		if k := strings.LastIndex(loc, "/tag/"); k >= 0 {
			if t := loc[k+len("/tag/"):]; t != "" {
				return t, loc, nil, nil
			}
		}
	}
	client := &http.Client{Timeout: 15 * time.Second}
	if resp, e := client.Get("https://github.com/haochan1996/tunnelport/releases"); e == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			if m := regexp.MustCompile(`releases/tag/([0-9A-Za-z._-]+)`).FindSubmatch(b); m != nil {
				t := string(m[1])
				return t, "https://github.com/haochan1996/tunnelport/releases/tag/" + t, nil, nil
			}
		}
	}
	resp, err := client.Get("https://api.github.com/repos/haochan1996/tunnelport/releases?per_page=10")
	if err != nil {
		return "", "", nil, fmt.Errorf("网络不可用：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 403 {
		return "", "", nil, fmt.Errorf("GitHub 接口限流，稍后重试；或手动更新：https://github.com/haochan1996/tunnelport/releases")
	}
	if resp.StatusCode != 200 {
		return "", "", nil, fmt.Errorf("GitHub 返回 %d，稍后重试", resp.StatusCode)
	}
	var rels []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rels); err != nil {
		return "", "", nil, fmt.Errorf("解析失败：%w", err)
	}
	for i := range rels {
		if !rels[i].Draft {
			return rels[i].TagName, rels[i].HTMLURL, &rels[i], nil
		}
	}
	return "", "", nil, fmt.Errorf("暂无发布版本")
}

// CheckUpdate 对比 GitHub Releases 最新版。
func (App) CheckUpdate() (UpdateDTO, error) {
	out := UpdateDTO{Current: version, Latest: version}
	tag, page, rel, err := latestTag()
	if err != nil {
		return out, err
	}
	if page == "" {
		page = "https://github.com/haochan1996/tunnelport/releases/tag/" + tag
	}
	out.Latest, out.ReleaseURL = tag, page
	out.HasUpdate = compareVersions(tag, version) > 0
	want := assetName()
	if rel != nil {
		for _, a := range rel.Assets {
			if a.Name == want {
				out.AssetName, out.AssetURL = a.Name, a.URL
				break
			}
		}
	} else {
		// 网页端点拿不到资产清单：按 release.yml 的命名约定直接构造直链
		out.AssetName = want
		out.AssetURL = "https://github.com/haochan1996/tunnelport/releases/download/" + tag + "/" + want
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
	tryGet := func(url string) (*http.Response, error) {
		resp, err := client.Get(url)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return resp, nil
	}
	resp, err := tryGet(u.AssetURL)
	if err != nil {
		// 国内网络回退 gh-proxy 镜像
		resp, err = tryGet("https://gh-proxy.com/" + u.AssetURL)
		if err != nil {
			return "", fmt.Errorf("下载失败：%v。可手动下载：%s", err, u.ReleaseURL)
		}
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
