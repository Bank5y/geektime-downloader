package gui

import (
	"errors"
	"strings"

	"github.com/nicoxiang/geektime-downloader/internal/config"
)

const maxConcurrentTasks = 8

// Settings are the download options editable in the GUI
type Settings struct {
	DownloadFolder         string `json:"downloadFolder"`
	Quality                string `json:"quality"`
	ColumnOutputType       int    `json:"columnOutputType"`
	DownloadComments       int    `json:"downloadComments"`
	PrintPDFWaitSeconds    int    `json:"printPDFWaitSeconds"`
	PrintPDFTimeoutSeconds int    `json:"printPDFTimeoutSeconds"`
	Interval               int    `json:"interval"`
	SkipEmbedVideo         bool   `json:"skipEmbedVideo"`
	Threads                int    `json:"threads"`
	ConcurrentTasks        int    `json:"concurrentTasks"`
	Overwrite              bool   `json:"overwrite"`
}

func settingsFromConfig(cfg config.AppConfig) Settings {
	return Settings{
		DownloadFolder:         cfg.DownloadFolder,
		Quality:                cfg.Quality,
		ColumnOutputType:       cfg.ColumnOutputType,
		DownloadComments:       cfg.DownloadComments,
		PrintPDFWaitSeconds:    cfg.PrintPDFWaitSeconds,
		PrintPDFTimeoutSeconds: cfg.PrintPDFTimeoutSeconds,
		Interval:               cfg.Interval,
		SkipEmbedVideo:         cfg.SkipEmbedVideo,
		Threads:                cfg.Threads,
		ConcurrentTasks:        1,
		Overwrite:              false,
	}
}

// apply returns a copy of base with settings applied
func (st Settings) apply(base config.AppConfig) config.AppConfig {
	base.DownloadFolder = st.DownloadFolder
	base.Quality = st.Quality
	base.ColumnOutputType = st.ColumnOutputType
	base.DownloadComments = st.DownloadComments
	base.PrintPDFWaitSeconds = st.PrintPDFWaitSeconds
	base.PrintPDFTimeoutSeconds = st.PrintPDFTimeoutSeconds
	base.Interval = st.Interval
	base.SkipEmbedVideo = st.SkipEmbedVideo
	base.Threads = st.Threads
	return base
}

func (st *Settings) validate(base config.AppConfig) error {
	st.DownloadFolder = strings.TrimSpace(st.DownloadFolder)
	if st.DownloadFolder == "" {
		return errors.New("下载目录不能为空")
	}
	if st.ConcurrentTasks < 1 || st.ConcurrentTasks > maxConcurrentTasks {
		return errors.New("同时下载任务数必须在 1 到 8 之间")
	}
	cfg := st.apply(base)
	return config.ValidateOptions(&cfg)
}
