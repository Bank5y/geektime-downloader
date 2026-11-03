# geektime-downloader

geektime-downloader 支持下载以下极客时间网站资源。

**极客时间**
- [x] 专栏(PDF/Markdown/音频)
- [x] 视频课
- [x] 每日一课
- [x] 大厂案例
- [x] 训练营视频
- [ ] 线下大会

**企业版极客时间**
- [ ] 体系课
- [ ] 每日一课
- [ ] 大厂案例
- [ ] 生态课
- [x] 训练营视频

部分资源暂未支持下载，欢迎PR。


[![go report card](https://goreportcard.com/badge/github.com/nicoxiang/geektime-downloader "go report card")](https://goreportcard.com/report/github.com/nicoxiang/geektime-downloader)
[![MIT license](https://img.shields.io/badge/license-MIT-brightgreen.svg)](https://opensource.org/licenses/MIT)

## Usage

### Prerequisites

- Chrome installed

### Install form source

```bash
# Go 1.16+
go install github.com/nicoxiang/geektime-downloader@latest

# Go version < 1.16
go get -u github.com/nicoxiang/geektime-downloader@latest
```

### Download binary files

See [release page](https://github.com/nicoxiang/geektime-downloader/releases)

### Sample

```bash
## Windows 为例
## Windows 推荐使用 Windows Terminal 打开

## cookie 方式登录
> geektime-downloader.exe --gcid "gcid" --gcess "gcess"

## 使用 8 个线程下载
> geektime-downloader.exe --gcid "gcid" --gcess "gcess" --threads 8
```

### 图形界面 (GUI)

```bash
## 启动图形界面, 会自动在默认浏览器中打开, 也可以在页面中填写 cookie 登录
> geektime-downloader.exe gui

## 启动时直接带上 cookie 和其他下载参数
> geektime-downloader.exe gui --gcid "gcid" --gcess "gcess" --threads 8
```

图形界面以内嵌网页的方式运行, 无需额外依赖:

- **全部课程**: 与官网同步的课程目录, 支持全站搜索, 按方向 (含二级标签)、类型、形式、会员课程筛选, 默认/最热/最新排序, 分页浏览
- **我的课程**: 已购买的课程 (含训练营), 支持关键词搜索、分类/形式/完结状态筛选、排序、分页; 也可以按课程 ID 直接打开
- **一键下载**: 课程封面右上角的下载按钮可直接下载整套课程, 无需进入详情页
- **课程详情**: 课程介绍 (你将获得 / 讲师介绍 / 课程介绍等) 与目录; 目录按章节分组, 支持搜索、分页、勾选部分文章或全部下载, 实时显示每篇文章的下载状态
- **训练营**: 视频课下载为视频, 图文文章保存为 Markdown
- **下载管理**: 任务队列分页显示进度、速度、剩余时间; 支持取消、重试、移除、打开所在目录, 批量重试失败/全部取消/清除已完成
- **设置**: 下载目录、清晰度、专栏输出格式、评论、**下载线程数**、**同时下载任务数**、任务间隔等

GUI 仅监听本机地址 (`127.0.0.1`, 随机端口), 所有接口均需要启动时生成的随机令牌, 请使用终端输出的地址打开。可选参数: `--addr` 指定监听地址, `--no-browser` 不自动打开浏览器。

### 多线程下载

`--threads` (GUI 中为「下载线程数」) 控制每个任务内的并行连接数, 默认 5, 范围 1-32:

- 视频课: m3u8 的 ts 分片并行下载, 最后按播放列表顺序合并
- 文中嵌入的 mp4 等大文件: 服务器支持 Range 时分块并行下载, 不支持时自动回退为单连接

GUI 中还可以设置「同时下载任务数」(1-8), 即同时下载的文章/视频数量。线程数和任务数过高可能触发极客时间限流, 请酌情设置。

### Help

```bash
## Windows 为例

> geektime-downloader.exe -h

Geektime-downloader is used to download geek time lessons

Usage:
  geektime-downloader [flags]
  geektime-downloader [command]

Available Commands:
  gui         启动图形界面 (在浏览器中打开)

Flags:
      --comments int            是否下载评论(0不下载,1下载首页评论,2下载所有评论) (default 1)
      --enterprise              是否下载企业版极客时间资源
  -f, --folder string           专栏和视频课的下载目标位置 (default "C:\\Users\\nico\\geektime-downloader")
      --gcess string            极客时间 cookie 值 gcess
      --gcid string             极客时间 cookie 值 gcid
  -h, --help                    help for geektime-downloader
      --interval int            下载资源的间隔时间, 单位为秒, 默认1秒 (default 1)
      --log-level string        日志记录级别(debug, info, warn, error, none) (default "info")
      --output int              专栏的输出内容(1pdf,2markdown,4audio)可自由组合 (default 1)
      --print-pdf-timeout int   Chrome生成PDF的超时时间, 单位为秒, 默认60秒 (default 60)
      --print-pdf-wait int      Chrome生成PDF前的等待页面加载时间, 单位为秒, 默认5秒 (default 5)
  -q, --quality string          下载视频清晰度(ld标清,sd高清,hd超清) (default "sd")
      --skip-embed-video        是否跳过下载图文课程中的视频
  -t, --threads int             下载线程数(1-32), 视频分片和文件分块并行下载 (default 5)
```

## Note

### 文件下载目标位置

文件下载目标位置可以通过 help 查看。默认情况下 Windows 位于 %USERPROFILE%/geektime-downloader 下；Unix, 包括 macOS, 位于 $HOME/geektime-downloader 下

### 如何查看课程 ID?

**普通课程：**

打开极客时间[课程列表页](https://time.geekbang.org/resource)，选择你想要查看的课程，在新打开的课程详情 Tab 页，查看 URL 最后的数字，例如下面的链接中 100056701 就是课程 ID：

```
https://time.geekbang.org/column/intro/100056701
```

**训练营课程：**

打开极客时间[训练营课程列表页](https://u.geekbang.org/schedule)，选择你想要查看的课程，在新打开的课程详情 Tab 页，查看 URL ```lesson/```后的数字，例如下面的链接中 419 就是课程 ID：

```
https://u.geekbang.org/lesson/419?article=535616
```

**每日一课课程：**

选择你想要下载的视频，查看 URL ```dailylesson/detail/```后的数字，例如下面的链接中 100122405 就是课程 ID：

```
https://time.geekbang.org/dailylesson/detail/100122405
```

**大厂案例课程：**

选择你想要下载的视频，查看 URL ```qconplus/detail/```后的数字，例如下面的链接中 100110494 就是课程 ID：

```
https://time.geekbang.org/qconplus/detail/100110494
```

**公开课课程：**

选择你想要下载的视频，查看 URL ```opencourse/intro/``` 或 ```opencourse/videointro/```后的数字，例如下面的链接中 100546701 就是课程 ID：

```
https://time.geekbang.org/opencourse/videointro/100546701
```

**其他：**

打开极客时间[我的课程-其他](https://time.geekbang.org/dashboard/course)，选择你想要查看的课程，在新打开的课程详情 Tab 页，查看 URL ```course/intro/``` 最后的数字，例如下面的链接中 100551201 就是课程 ID：

```
https://time.geekbang.org/course/intro/100551201
```

**企业版训练营：**

选择你想要查看的课程，查看 URL ```mall/product/```后的数字，例如下面的链接中 100618109 就是课程 ID：

```
https://b.geekbang.org/mall/product/100618109
```

### 为什么我下载的PDF是空白页?
首先下载课程请保证VPN已关闭。在此前提下如果仍然出现空白页情况，说明后台Chrome网页加载速度较慢，可以尝试加大--print-pdf-wait参数，保证页面完全加载完成后再开始生成PDF。

### 为什么我下载PDF一直提示超时?
首先下载课程请保证VPN已关闭。在此前提下如果下载持续出现超时，有可能是因为课程章节图片等内容较多，生成速度慢，比如课程《AI 绘画核心技术与实战》中的部分章节，可以尝试加大--print-pdf-timeout参数，并耐心等待。

### 如何下载专栏的 Markdown 格式和文章音频?

默认情况下载专栏的输出内容只有 PDF，可以通过 --output 参数按需选择是否需要下载 Markdown 格式和文章音频。比如 --output 3 就是下载 PDF 和 Markdown；--output 6 就是下载 Markdown 和音频；--output 7 就是下载所有。

Markdown 格式虽然显示效果上不及 PDF，但优势为可以显示完整的代码块（PDF 代码块在水平方向太长时会有缺失）并保留了原文中的超链接。

现在部分新课程的专栏文章中会包含视频，如课程《Kubernetes 入门实战课》等，目前程序会自动下载文章所包含的视频，视频目录在文章所在目录的子目录 videos 下，此类文章PDF的下载会耗费更多时间，请耐心等待。

### 退出程序和继续下载

Ctrl + C 退出程序。如果选择“下载所有”后中断程序，可重新进入程序继续下载。
