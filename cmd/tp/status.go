package main

import (
	"fmt"
	"github.com/haochan1996/tunnelport/internal/address"
	"github.com/haochan1996/tunnelport/internal/certs"
	"github.com/haochan1996/tunnelport/internal/tokenx"
	"os"

	"github.com/spf13/cobra"

	"github.com/haochan1996/tunnelport/internal/config"
)

func statusCmd() *cobra.Command {
	var flagShow bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "查看当前状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := config.LoadState()
			if err != nil {
				return err
			}
			if s == nil {
				fmt.Println("还没有运行过。先在 VPS 上执行 tp serve。")
				return nil
			}
			fmt.Printf("角色:      %s\n", s.Role)
			fmt.Printf("\n入口地址（复制后粘贴到 Orca 桌面端「自定义网络地址」）:\n   %s\n", s.Entry)
			fmt.Printf("控制通道:  %s\n", s.Control)
			fmt.Printf("token 指纹: %s\n", s.TokenFP)
			fmt.Printf("版本:      %s\n", s.Version)
			fmt.Printf("更新时间:  %s\n", s.UpdatedAt)
			if flagShow {
				token, err := config.LoadToken()
				if err != nil || token == "" {
					fmt.Println("（token 未保存：重新运行 tp serve，或用 --regenerate 重置）")
					return nil
				}
				dir, _ := config.Dir()
				fp, _, err := certs.FingerprintFile(dir + string(os.PathSeparator) + "cert.pem")
				if err != nil {
					fmt.Println("读取证书失败:", err)
					return nil
				}
				host, ctrl := splitHostPort(s.Control)
				_, pub := splitHostPort(s.Entry)
				u := tokenx.PairURL(host, ctrl, token, fp, pub)
				fmt.Printf("\n配对链接（桌面机 tp connect 用，随时可在此取回）:\n  %s\n\n%s\n", u, address.RenderQR(u))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&flagShow, "show-pairing", false, "显示完整配对链接与二维码（含 token，勿外泄）")
	return cmd
}
