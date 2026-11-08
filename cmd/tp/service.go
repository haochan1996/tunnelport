package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func serviceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "安装 / 卸载开机自启（W2 提供）",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "安装自启",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("tp service install 将在 W2 提供（launchd / systemd / 计划任务）")
		},
	}, &cobra.Command{
		Use:   "uninstall",
		Short: "卸载自启",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("tp service uninstall 将在 W2 提供")
		},
	})
	return cmd
}
