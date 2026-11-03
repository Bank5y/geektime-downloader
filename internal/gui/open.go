package gui

import (
	"os/exec"
	"runtime"
)

// openURL opens url in the default browser
func openURL(url string) error {
	switch runtime.GOOS {
	case "windows":
		return start("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		return start("open", url)
	default:
		return start("xdg-open", url)
	}
}

// openPath opens a local directory in the system file manager
func openPath(path string) error {
	switch runtime.GOOS {
	case "windows":
		return start("explorer", path)
	case "darwin":
		return start("open", path)
	default:
		return start("xdg-open", path)
	}
}

func start(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	// reap the child process
	go func() { _ = cmd.Wait() }()
	return nil
}
