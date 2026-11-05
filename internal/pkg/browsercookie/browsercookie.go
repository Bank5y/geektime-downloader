// Package browsercookie reads cookies of a site from the Chromium based browsers
// (Chrome / Edge / Brave / Chromium) installed on this machine.
//
// Cookie values are encrypted at rest with a per-OS scheme (keyring, keychain,
// DPAPI, app-bound encryption...). Instead of re-implementing all of them, the
// cookie database of a profile is copied into a temporary user data directory
// and opened by a headless instance of the same browser, which decrypts it
// itself; the cookies are then read over the DevTools protocol. The original
// profile is never opened or modified.
package browsercookie

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// ErrFileLocked is returned when the cookie database can't be copied, usually
// because the browser holds an exclusive lock on it (Windows).
var ErrFileLocked = errors.New("无法读取浏览器的 Cookie 文件, 请关闭该浏览器后重试")

// Source is one profile of an installed browser.
type Source struct {
	ID          string `json:"id"`      // browser key + "/" + profile dir
	Browser     string `json:"browser"` // display name, e.g. Google Chrome
	Profile     string `json:"profile"` // display name of the profile
	profileDir  string // e.g. Default, Profile 1
	userDataDir string
	execPath    string
	cookieRel   string // cookie db path relative to the profile dir
}

func (s Source) String() string {
	if s.Profile == "" {
		return s.Browser
	}
	return s.Browser + " · " + s.Profile
}

type browser struct {
	key, name   string
	userDataDir string
	execs       []string // absolute paths or names looked up in PATH
}

func candidates() []browser {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		pf, pf86 := os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")
		in := func(rel string) []string {
			var out []string
			for _, base := range []string{pf, pf86, local} {
				if base != "" {
					out = append(out, filepath.Join(base, rel))
				}
			}
			return out
		}
		return []browser{
			{"chrome", "Google Chrome", filepath.Join(local, `Google\Chrome\User Data`), in(`Google\Chrome\Application\chrome.exe`)},
			{"edge", "Microsoft Edge", filepath.Join(local, `Microsoft\Edge\User Data`), in(`Microsoft\Edge\Application\msedge.exe`)},
			{"brave", "Brave", filepath.Join(local, `BraveSoftware\Brave-Browser\User Data`), in(`BraveSoftware\Brave-Browser\Application\brave.exe`)},
			{"chromium", "Chromium", filepath.Join(local, `Chromium\User Data`), in(`Chromium\Application\chrome.exe`)},
		}
	case "darwin":
		as := filepath.Join(home, "Library", "Application Support")
		app := func(name, bin string) []string {
			return []string{
				filepath.Join("/Applications", name+".app", "Contents", "MacOS", bin),
				filepath.Join(home, "Applications", name+".app", "Contents", "MacOS", bin),
			}
		}
		return []browser{
			{"chrome", "Google Chrome", filepath.Join(as, "Google", "Chrome"), app("Google Chrome", "Google Chrome")},
			{"edge", "Microsoft Edge", filepath.Join(as, "Microsoft Edge"), app("Microsoft Edge", "Microsoft Edge")},
			{"brave", "Brave", filepath.Join(as, "BraveSoftware", "Brave-Browser"), app("Brave Browser", "Brave Browser")},
			{"chromium", "Chromium", filepath.Join(as, "Chromium"), app("Chromium", "Chromium")},
		}
	default:
		cfg, err := os.UserConfigDir()
		if err != nil {
			cfg = filepath.Join(home, ".config")
		}
		return []browser{
			{"chrome", "Google Chrome", filepath.Join(cfg, "google-chrome"), []string{"google-chrome", "google-chrome-stable"}},
			{"chrome-beta", "Google Chrome Beta", filepath.Join(cfg, "google-chrome-beta"), []string{"google-chrome-beta"}},
			{"edge", "Microsoft Edge", filepath.Join(cfg, "microsoft-edge"), []string{"microsoft-edge", "microsoft-edge-stable"}},
			{"brave", "Brave", filepath.Join(cfg, "BraveSoftware", "Brave-Browser"), []string{"brave-browser", "brave"}},
			{"chromium", "Chromium", filepath.Join(cfg, "chromium"), []string{"chromium", "chromium-browser"}},
		}
	}
}

func findExec(execs []string) string {
	for _, e := range execs {
		if filepath.IsAbs(e) {
			if st, err := os.Stat(e); err == nil && !st.IsDir() {
				return e
			}
			continue
		}
		if p, err := exec.LookPath(e); err == nil {
			return p
		}
	}
	return ""
}

// cookieDB returns the cookie database path relative to the profile dir
func cookieDB(profileDir string) string {
	for _, rel := range []string{filepath.Join("Network", "Cookies"), "Cookies"} {
		if st, err := os.Stat(filepath.Join(profileDir, rel)); err == nil && !st.IsDir() {
			return rel
		}
	}
	return ""
}

// Sources lists the browser profiles on this machine that have a cookie database
// and whose browser executable can be found.
func Sources() []Source {
	var out []Source
	for _, b := range candidates() {
		if st, err := os.Stat(b.userDataDir); err != nil || !st.IsDir() {
			continue
		}
		execPath := findExec(b.execs)
		if execPath == "" {
			continue
		}
		for _, p := range profiles(b.userDataDir) {
			rel := cookieDB(filepath.Join(b.userDataDir, p.dir))
			if rel == "" {
				continue
			}
			out = append(out, Source{
				ID:          b.key + "/" + p.dir,
				Browser:     b.name,
				Profile:     p.name,
				profileDir:  p.dir,
				userDataDir: b.userDataDir,
				execPath:    execPath,
				cookieRel:   rel,
			})
		}
	}
	return out
}

type profile struct{ dir, name string }

// profiles reads the profile list from "Local State", most recently used first
func profiles(userDataDir string) []profile {
	var st struct {
		Profile struct {
			InfoCache map[string]struct {
				Name       string  `json:"name"`
				ActiveTime float64 `json:"active_time"`
			} `json:"info_cache"`
			LastUsed string `json:"last_used"`
		} `json:"profile"`
	}
	var out []profile
	if b, err := os.ReadFile(filepath.Join(userDataDir, "Local State")); err == nil && json.Unmarshal(b, &st) == nil {
		active := map[string]float64{}
		for dir, info := range st.Profile.InfoCache {
			out = append(out, profile{dir, info.Name})
			active[dir] = info.ActiveTime
		}
		sort.SliceStable(out, func(i, j int) bool {
			if (out[i].dir == st.Profile.LastUsed) != (out[j].dir == st.Profile.LastUsed) {
				return out[i].dir == st.Profile.LastUsed
			}
			return active[out[i].dir] > active[out[j].dir]
		})
	}
	if len(out) == 0 {
		out = append(out, profile{"Default", ""})
	}
	return out
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Read returns the cookies sent to rawURL whose name is one of names, read from the
// given browser profile. Values are never logged.
func Read(ctx context.Context, src Source, rawURL string, names ...string) (map[string]string, error) {
	tmp, err := os.MkdirTemp("", "gd-browser-")
	if err != nil {
		return nil, err
	}
	// the copy holds the user's cookies, always remove it
	defer removeAll(tmp)

	// Local State holds the cookie encryption key on Windows / app-bound encryption
	if _, err := os.Stat(filepath.Join(src.userDataDir, "Local State")); err == nil {
		if err := copyFile(filepath.Join(src.userDataDir, "Local State"), filepath.Join(tmp, "Local State")); err != nil {
			return nil, err
		}
	}
	db := filepath.Join(src.userDataDir, src.profileDir, src.cookieRel)
	dst := filepath.Join(tmp, src.profileDir, src.cookieRel)
	if err := copyFile(db, dst); err != nil {
		return nil, fmt.Errorf("%w (%v)", ErrFileLocked, err)
	}
	if _, err := os.Stat(db + "-journal"); err == nil {
		_ = copyFile(db+"-journal", dst+"-journal")
	}

	// no "password-store=basic" / "use-mock-keychain": the browser must use the
	// real OS key store to decrypt the copied cookies
	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(src.execPath),
		chromedp.UserDataDir(tmp),
		chromedp.Flag("profile-directory", src.profileDir),
		chromedp.Headless,
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.DisableGPU,
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-component-update", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("mute-audio", true),
	}
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()
	// close the browser and wait for it to exit before the temp dir is removed
	defer func() {
		_ = chromedp.Cancel(browserCtx)
		allocCancel()
	}()

	var cookies []*network.Cookie
	err = chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		cookies, err = network.GetCookies().WithURLs([]string{rawURL}).Do(ctx)
		return err
	}))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("启动 %s 超时", src.Browser)
		}
		return nil, fmt.Errorf("启动 %s 失败: %v", src.Browser, err)
	}

	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	out := make(map[string]string)
	for _, c := range cookies {
		if want[c.Name] && strings.TrimSpace(c.Value) != "" {
			out[c.Name] = c.Value
		}
	}
	return out, nil
}

// removeAll removes dir, retrying briefly while browser processes release their files
func removeAll(dir string) {
	for i := 0; i < 10; i++ {
		if err := os.RemoveAll(dir); err == nil {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
}
