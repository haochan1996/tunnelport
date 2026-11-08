package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/haochan1996/tunnelport-internal/internal/config"
)

func logsCmd() *cobra.Command {
	var flagFollow bool
	var flagLines int

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "查看引擎日志（rathole 输出，含 TLS/连接错误）",
		Long: `查看引擎日志。日志按大小滚动（5MB → .old），保留最近两份。
日志已脱敏：不含 token 明文与终端内容（FR-9）。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := config.Dir()
			if err != nil {
				return err
			}
			logPath := filepath.Join(dir, "engine.log")
			if _, err := os.Stat(logPath); os.IsNotExist(err) {
				fmt.Println("暂无引擎日志（tp 还没建立过隧道，或日志已被滚动）。")
				return nil
			}
			if flagFollow {
				return followFile(logPath)
			}
			return tailFile(logPath, flagLines)
		},
	}
	cmd.Flags().BoolVarP(&flagFollow, "follow", "f", false, "持续跟踪新日志（类似 tail -f）")
	cmd.Flags().IntVarP(&flagLines, "lines", "n", 50, "显示末尾行数")
	return cmd
}

// tailFile 输出文件末尾 n 行（简单实现：全文读入后切尾；日志上限 5MB，可接受）。
func tailFile(path string, n int) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := splitLines(b)
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for _, l := range lines {
		fmt.Println(l)
	}
	return nil
}

// followFile 轮询追加内容（MVP 不引入 fsnotify 依赖；Windows 兼容性优先）。
func followFile(path string) error {
	var offset int64
	if st, err := os.Stat(path); err == nil {
		offset = st.Size()
	}
	fmt.Println("跟踪中，Ctrl-C 退出…")
	buf := make([]byte, 4096)
	for {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		if st, err := f.Stat(); err == nil && st.Size() > offset {
			_, _ = f.Seek(offset, 0)
			for {
				n, rerr := f.Read(buf)
				if n > 0 {
					fmt.Print(string(buf[:n]))
				}
				if rerr != nil || n < len(buf) {
					offset += int64(n)
					break
				}
			}
		}
		f.Close()
		time.Sleep(500 * time.Millisecond)
	}
}

func splitLines(b []byte) []string {
	var out []string
	start := 0
	for i, c := range b {
		if c == '\n' {
			out = append(out, string(b[start:i]))
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, string(b[start:]))
	}
	return out
}
