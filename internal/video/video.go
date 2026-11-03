package video

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cheggaaa/pb/v3"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/nicoxiang/geektime-downloader/internal/geektime"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/crypto"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/downloader"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/filenamify"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/files"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/logger"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/m3u8"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/progress"
	"github.com/nicoxiang/geektime-downloader/internal/video/vod"
)

const (
	// syncByte = uint8(71) //0x47

	// TSExtension ...
	TSExtension = ".ts"
)

// EncryptType enum
type EncryptType int

// const (
// 	// AliyunVodEncrypt ...
// 	AliyunVodEncrypt EncryptType = iota
// 	// HLSStandardEncrypt ...
// 	HLSStandardEncrypt
// )

// GetPlayInfoResponse is the response struct for api GetPlayInfo
type GetPlayInfoResponse struct {
	RequestID    string                        `json:"RequestId" xml:"RequestId"`
	VideoBase    vod.VideoBase                 `json:"VideoBase" xml:"VideoBase"`
	PlayInfoList vod.PlayInfoListInGetPlayInfo `json:"PlayInfoList" xml:"PlayInfoList"`
}

// DownloadArticleVideo download normal video cource ...
// sourceType: normal video cource 1
func DownloadArticleVideo(ctx context.Context,
	client *geektime.Client,
	articleID int,
	sourceType int,
	projectDir string,
	quality string,
	concurrency int,
) error {
	logger.Infof("Begin download normal article video, articleID: %d, sourceType: %d", articleID, sourceType)
	articleInfo, err := client.V3ArticleInfo(articleID)
	if err != nil {
		return err
	}
	if articleInfo.Data.Info.Video.ID == "" {
		return nil
	}
	playAuth, err := client.VideoPlayAuth(articleInfo.Data.Info.ID, sourceType, articleInfo.Data.Info.Video.ID)
	if err != nil {
		return err
	}
	err = downloadAliyunVodEncryptVideo(ctx,
		client,
		playAuth,
		articleInfo.Data.Info.Title,
		projectDir,
		quality,
		articleInfo.Data.Info.Video.ID,
		concurrency)
	if err != nil {
		logger.Errorf(err, "Download normal article video failed, articleID: %d", articleID)
		return err
	} else {
		logger.Infof("Finish download normal article video, articleID: %d", articleID)
		return nil
	}
}

// DownloadEnterpriseArticleVideo download enterprise video
func DownloadEnterpriseArticleVideo(ctx context.Context,
	client *geektime.Client,
	articleID int,
	projectDir string,
	quality string,
	concurrency int,
) error {
	logger.Infof("Begin download enterprise article video, articleID: %d", articleID)
	articleInfo, err := client.V1EnterpriseArticleDetail(strconv.Itoa(articleID))
	if err != nil {
		return err
	}
	if articleInfo.Data.Video.ID == "" {
		return nil
	}
	playAuth, err := client.EnterpriseVideoPlayAuth(strconv.Itoa(articleID), articleInfo.Data.Video.ID)
	if err != nil {
		return err
	}
	err = downloadAliyunVodEncryptVideo(ctx,
		client,
		playAuth,
		articleInfo.Data.Article.Title,
		projectDir,
		quality,
		articleInfo.Data.Video.ID,
		concurrency)
	if err != nil {
		logger.Errorf(err, "Download enterprise article video failed, articleID: %d", articleID)
		return err
	} else {
		logger.Infof("Finish download enterprise article video, articleID: %d", articleID)
		return nil
	}
}

// DownloadUniversityVideo ...
func DownloadUniversityVideo(ctx context.Context,
	client *geektime.Client,
	articleID int,
	currentProduct geektime.Course,
	projectDir string,
	quality string,
	concurrency int,
) error {
	logger.Infof("Begin download university article video, articleID: %d", articleID)
	playAuthInfo, err := client.UniversityVideoPlayAuth(articleID, currentProduct.ID)
	if err != nil {
		return err
	}

	videoTitle := getUniversityVideoTitle(articleID, currentProduct)
	err = downloadAliyunVodEncryptVideo(ctx,
		client,
		playAuthInfo.Data.PlayAuth,
		videoTitle,
		projectDir,
		quality,
		playAuthInfo.Data.VID,
		concurrency)
	if err != nil {
		logger.Errorf(err, "Download university article video failed, articleID: %d", articleID)
		return err
	} else {
		logger.Infof("Finish download university article video, articleID: %d", articleID)
		return nil
	}
}

func downloadAliyunVodEncryptVideo(ctx context.Context,
	client *geektime.Client,
	playAuth,
	videoTitle,
	projectDir,
	quality,
	videoID string,
	concurrency int,
) error {
	clientRand := uuid.NewString()
	playInfoURL, err := vod.BuildVodGetPlayInfoURL(playAuth, videoID, clientRand)
	if err != nil {
		return err
	}
	playInfo, err := getPlayInfo(client, playInfoURL, quality)
	if err != nil {
		return err
	}
	tsURLPrefix := extractTSURLPrefix(playInfo.PlayURL)

	tsFileNames, isVodEncryptVideo, err := m3u8.Parse(client, playInfo.PlayURL)
	if err != nil {
		return err
	}

	decryptKey := ""
	if isVodEncryptVideo {
		decryptKey = crypto.GetAESDecryptKey(clientRand, playInfo.Rand, playInfo.Plaintext)
	}
	return download(ctx, tsURLPrefix, videoTitle, projectDir, tsFileNames, []byte(decryptKey), playInfo.Size, isVodEncryptVideo, concurrency)
}

// DownloadMP4 download MP4 resources in article
func DownloadMP4(ctx context.Context, title, projectDir string, mp4URLs []string, overwrite bool, concurrency int) (err error) {
	logger.Infof("Begin download article mp4 videos, title: %s, mp4URLs: %v", title, mp4URLs)
	progress.Stage(ctx, "下载文中视频")
	filenamifyTitle := filenamify.Filenamify(title)
	videoDir := filepath.Join(projectDir, "videos", filenamifyTitle)
	if err = os.MkdirAll(videoDir, os.ModePerm); err != nil {
		return
	}

	for _, mp4URL := range mp4URLs {
		u, _ := url.Parse(mp4URL)
		dst := filepath.Join(videoDir, path.Base(u.Path))

		if files.CheckFileExists(dst) && !overwrite {
			continue
		}

		headers := make(map[string]string, 2)
		headers[geektime.Origin] = geektime.DefaultBaseURL
		headers[geektime.UserAgent] = geektime.DefaultUserAgent
		logger.Infof("Begin download single article mp4 video, title: %s, mp4URL: %s", title, mp4URL)
		_, err := downloader.DownloadFileConcurrently(ctx, dst, mp4URL, headers, concurrency)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Errorf(err, "Failed to download single article mp4 video, title: %s, mp4URL: %s", title, mp4URL)
			return nil
		}
		logger.Infof("Finish download single article mp4 video, title: %s, mp4URL: %s", title, mp4URL)
	}
	logger.Infof("Finish download all article mp4 videos, title: %s, mp4URLs: %v", title, mp4URLs)
	return
}

func download(ctx context.Context,
	tsURLPrefix,
	title,
	projectDir string,
	tsFileNames []string,
	decryptKey []byte,
	size int64,
	isVodEncryptVideo bool,
	concurrency int,
) (err error) {
	// Make temp ts folder and download temp ts files
	filenamifyTitle := filenamify.Filenamify(title)
	tempVideoDir := filepath.Join(projectDir, filenamifyTitle)
	if err = os.MkdirAll(tempVideoDir, os.ModePerm); err != nil {
		return
	}
	// temp folder cleanup
	defer func() {
		_ = os.RemoveAll(tempVideoDir)
	}()

	reporter := progress.FromContext(ctx)
	if reporter == nil {
		bar := newBar(size, fmt.Sprintf("[正在下载 %s] ", filenamifyTitle))
		bar.Start()
		defer bar.Finish()
		reporter = &barReporter{bar: bar}
	}
	reporter.SetStage("下载视频")
	reporter.SetTotal(size)

	if concurrency <= 0 {
		concurrency = 1
	}

	headers := map[string]string{
		geektime.Origin:    geektime.DefaultBaseURL,
		geektime.UserAgent: geektime.DefaultUserAgent,
	}

	// download ts segments in parallel, each segment with a single connection
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)
	for _, tsFileName := range tsFileNames {
		if gctx.Err() != nil {
			break
		}
		u := tsURLPrefix + tsFileName
		dst := filepath.Join(tempVideoDir, tsFileName)
		g.Go(func() error {
			fileSize, err := downloader.DownloadFileConcurrently(gctx, dst, u, headers, 1)
			if err != nil {
				return err
			}
			reporter.Add(fileSize)
			return nil
		})
	}
	if err = g.Wait(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}

	reporter.SetStage("合并视频")
	// Read temp ts files, decrypt and merge into the one final video file
	err = mergeTSFiles(tempVideoDir, tsFileNames, filenamifyTitle, projectDir, decryptKey, isVodEncryptVideo)

	return
}

// mergeTSFiles merges ts segments in playlist order into one final video file.
func mergeTSFiles(tempVideoDir string, tsFileNames []string, filenamifyTitle, projectDir string, key []byte, isVodEncryptVideo bool) error {
	fullPath := filepath.Join(projectDir, filenamifyTitle+TSExtension)
	finalVideoFile, err := os.OpenFile(fullPath, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}
	removeOnError := false
	defer func() {
		_ = finalVideoFile.Close()
		if removeOnError {
			_ = os.Remove(fullPath)
		}
	}()
	for _, tsFileName := range tsFileNames {
		f, err := os.ReadFile(filepath.Join(tempVideoDir, tsFileName))
		if err != nil {
			removeOnError = true
			return err
		}

		if isVodEncryptVideo {
			tsParser, err := m3u8.NewTSParser(f, string(key))
			if err != nil {
				removeOnError = true
				return err
			}
			f, err = tsParser.Decrypt()
			if err != nil {
				removeOnError = true
				return err
			}
		}

		if _, err := finalVideoFile.Write(f); err != nil {
			removeOnError = true
			return err
		}
	}
	return nil
}

// barReporter adapts a terminal progress bar to progress.Reporter
type barReporter struct {
	mu  sync.Mutex
	bar *pb.ProgressBar
}

func (b *barReporter) SetStage(string) {}

func (b *barReporter) SetTotal(total int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bar.SetTotal(total)
}

func (b *barReporter) Add(n int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	addBarValue(b.bar, n)
}

func newBar(size int64, prefix string) *pb.ProgressBar {
	bar := pb.New64(size)
	bar.SetRefreshRate(time.Second)
	bar.Set(pb.Bytes, true)
	bar.Set(pb.SIBytesPrefix, true)
	bar.SetTemplate(pb.Simple)
	bar.Set("prefix", prefix)
	return bar
}

// total bytes may greater than expected
func addBarValue(bar *pb.ProgressBar, written int64) {
	if bar.Current()+written > bar.Total() {
		bar.SetCurrent(bar.Total())
	} else {
		bar.Add64(written)
	}
}

func getUniversityVideoTitle(articleID int, currentProduct geektime.Course) string {
	for _, v := range currentProduct.Articles {
		if v.AID == articleID {
			return v.Title
		}
	}
	return ""
}

func extractTSURLPrefix(m3u8url string) string {
	i := strings.LastIndex(m3u8url, "/")
	return m3u8url[:i+1]
}

func getPlayInfo(client *geektime.Client, playInfoURL, quality string) (vod.PlayInfo, error) {
	var getPlayInfoResp GetPlayInfoResponse
	var playInfo vod.PlayInfo
	_, err := client.RestyClient.R().
		SetResult(&getPlayInfoResp).
		Get(playInfoURL)
	if err != nil {
		return playInfo, err
	}

	playInfoList := getPlayInfoResp.PlayInfoList.PlayInfo
	for _, p := range playInfoList {
		if strings.EqualFold(p.Definition, quality) {
			playInfo = p
		}
	}
	return playInfo, nil
}
