package main

import (
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
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

// latestTag 取最新非 draft 版本。多源收集、取最大版本号——
// 单一来源可能被代理缓存或发布后索引延迟返回旧值（v1.0.5 加固）。
func latestTag() (tag, pageURL string, rel *ghRelease, err error) {
	var cands []string
	noRedirect := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	// 1. /releases/latest 重定向（浏览器口径）
	if resp, e := noRedirect.Get("https://github.com/haochan1996/tunnelport/releases/latest"); e == nil {
		loc := resp.Header.Get("Location")
		resp.Body.Close()
		if k := strings.LastIndex(loc, "/tag/"); k >= 0 {
			if t := loc[k+len("/tag/"):]; t != "" {
				cands = append(cands, t)
				pageURL = loc
			}
		}
	}
	// 2. releases 列表页：收集全部 tag 链接（多匹配取最大，不依赖顺序）
	client := &http.Client{Timeout: 15 * time.Second}
	if resp, e := client.Get("https://github.com/haochan1996/tunnelport/releases"); e == nil {
		if resp.StatusCode == 200 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			for _, m := range regexp.MustCompile(`releases/tag/([0-9A-Za-z._-]+)`).FindAllSubmatch(b, -1) {
				cands = append(cands, string(m[1]))
			}
		}
		resp.Body.Close()
	}
	// 3. API 回退（匿名配额有限）
	var apiRel *ghRelease
	if resp, e := client.Get("https://api.github.com/repos/haochan1996/tunnelport/releases?per_page=10"); e == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var rels []ghRelease
			if json.NewDecoder(resp.Body).Decode(&rels) == nil {
				for i := range rels {
					if !rels[i].Draft {
						cands = append(cands, rels[i].TagName)
						if apiRel == nil {
							apiRel = &rels[i]
						}
						break
					}
				}
			}
		}
	}
	if len(cands) == 0 {
		return "", "", nil, fmt.Errorf("网络不可用或 GitHub 暂不可达，稍后重试；也可手动更新：https://github.com/haochan1996/tunnelport/releases")
	}
	// 取最大版本
	best := cands[0]
	for _, c := range cands[1:] {
		if compareVersions(c, best) > 0 {
			best = c
		}
	}
	if pageURL == "" {
		pageURL = "https://github.com/haochan1996/tunnelport/releases/tag/" + best
	}
	// 只有 API 候选就是最优解时才带资产清单（否则按命名约定构造直链）
	if apiRel != nil && apiRel.TagName == best {
		return best, pageURL, apiRel, nil
	}
	return best, pageURL, nil, nil
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

// ---------- 更新进度（v1.0.12）：后台执行 + 轮询 ----------

type updateState struct {
	mu      sync.Mutex
	running bool
	stage   string // downloading / installing / done / error
	percent int    // -1 = 总长未知
	message string // 速率文本 / 错误信息 / 完成提示
}

var upd = updateState{percent: -1}

// progressReader 边下边上报（每 ~250ms）。
type progressReader struct {
	r     io.Reader
	total int64
	read  int64
	last  time.Time
	lastN int64
	fn    func(read, total int64, speed string)
}

func (pr *progressReader) Read(b []byte) (int, error) {
	n, err := pr.r.Read(b)
	pr.read += int64(n)
	if now := time.Since(pr.last); now > 250*time.Millisecond {
		speed := float64(pr.read-pr.lastN) / now.Seconds()
		pr.last, pr.lastN = time.Now(), pr.read
		pr.fn(pr.read, pr.total, fmt.Sprintf("%.1f MB/s", speed/1e6))
	}
	return n, err
}

// StartUpdate 启动后台更新（幂等：进行中直接返回）。
func (App) StartUpdate() (string, error) {
	upd.mu.Lock()
	if upd.running {
		upd.mu.Unlock()
		return "", nil
	}
	upd.running, upd.stage, upd.percent, upd.message = true, "downloading", -1, ""
	upd.mu.Unlock()
	go func() {
		msg, err := runUpdate()
		upd.mu.Lock()
		defer upd.mu.Unlock()
		upd.running = false
		if err != nil {
			upd.stage, upd.message = "error", err.Error()
			return
		}
		upd.stage, upd.message = "done", msg
	}()
	return "", nil
}

// UpdateProgress 查询更新进度（界面轮询）。
func (App) UpdateProgress() (map[string]any, error) {
	upd.mu.Lock()
	defer upd.mu.Unlock()
	return map[string]any{
		"running": upd.running,
		"stage":   upd.stage,
		"percent": upd.percent,
		"message": upd.message,
	}, nil
}

// runUpdate：检查 → 下载（带进度）→ 原位替换。
func runUpdate() (string, error) {
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
		resp, err = tryGet("https://gh-proxy.com/" + u.AssetURL)
		if err != nil {
			return "", fmt.Errorf("下载失败：%v。可手动下载：%s", err, u.ReleaseURL)
		}
	}
	defer resp.Body.Close()
	pr := &progressReader{r: resp.Body, total: resp.ContentLength, last: time.Now(),
		fn: func(read, total int64, speed string) {
			upd.mu.Lock()
			if total > 0 {
				upd.percent = int(read * 100 / total)
			}
			upd.message = speed
			upd.mu.Unlock()
		}}
	exe := exePath
	if exe == "" {
		exe, _ = os.Executable()
	}
	tmp := exe + ".new"
	f, err := os.Create(tmp)
	if err != nil {
		return "", fmt.Errorf("写入失败：%w", err)
	}
	if _, err := io.Copy(f, pr); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return "", fmt.Errorf("下载中断：%w", err)
	}
	f.Close()
	upd.mu.Lock()
	upd.stage = "installing"
	upd.mu.Unlock()
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("替换失败（%v）。可手动下载：%s", err, u.ReleaseURL)
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Rename(old, exe)
		return "", fmt.Errorf("替换失败（%v）", err)
	}
	_ = os.Remove(old)
	return "已更新到 " + u.Latest + "，重启应用生效", nil
}

// DoUpdate 同步版更新（界面已改用 StartUpdate/UpdateProgress，保留兼容）。
func (App) DoUpdate() (string, error) {
	return runUpdate()
}

// Restart 重启应用 —— 稳定版（v1.0.31）：
// 直接拉起新进程（不经 PowerShell/cmd 中间人，消灭转义/延时/临时脚本三个故障源），
// 新进程带 --restart 轮询等单实例锁；旧进程优雅退出 + 强制退出兜底，保证锁必释放。
func (App) Restart() error {
	exe := exePath
	if exe == "" {
		exe, _ = os.Executable()
	}
	cmd := hiddenCmd(exe, "--restart")
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("拉起新进程失败：%w（可手动重开程序）", err)
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		quitting = true
		mygo.App.Quit()
		// 兜底：webview 拆解偶发卡死导致进程不退 → 强制退出，互斥体必释放
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}()
	return nil
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
