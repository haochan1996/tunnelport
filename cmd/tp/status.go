package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/haochan1996/tunnelport-internal/internal/config"
)

func statusCmd() *cobra.Command {
	return &cobra.Command{
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
			return nil
		},
	}
}
