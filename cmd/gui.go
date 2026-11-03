package cmd

import (
	"github.com/spf13/cobra"

	"github.com/nicoxiang/geektime-downloader/internal/config"
	"github.com/nicoxiang/geektime-downloader/internal/gui"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/logger"
)

var guiOpts gui.Options

func init() {
	guiCmd.Flags().StringVar(&guiOpts.Addr, "addr", "127.0.0.1:0", "图形界面监听地址, 默认仅本机访问并随机选择端口")
	var noBrowser bool
	guiCmd.Flags().BoolVar(&noBrowser, "no-browser", false, "启动后不自动打开浏览器")
	guiCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		guiOpts.OpenBrowser = !noBrowser
		logger.Init(cfg.LogLevel)
		// cookies are optional here, they can be entered in the GUI
		return config.ValidateOptions(&cfg)
	}
	rootCmd.AddCommand(guiCmd)
}

var guiCmd = &cobra.Command{
	Use:          "gui",
	Short:        "启动图形界面 (在浏览器中打开)",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return gui.Run(cmd.Context(), cfg, guiOpts)
	},
}
