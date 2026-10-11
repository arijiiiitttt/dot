package src

import (
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// Helpers that exist in newer Go releases, kept here so the code also builds
// with the old Go 1.10 toolchain used for the Windows XP build.

func replaceAll(s, old, new string) string { return strings.Replace(s, old, new, -1) }

func exitCodeOf(e *exec.ExitError) int {
	if ws, ok := e.Sys().(syscall.WaitStatus); ok {
		return ws.ExitStatus()
	}
	return 1
}

func userHome() (string, error) {
	for _, k := range []string{"USERPROFILE", "HOME"} {
		if v := os.Getenv(k); v != "" {
			return v, nil
		}
	}
	return "", os.ErrNotExist
}

func readDir(p string) ([]os.FileInfo, error) { return ioutil.ReadDir(p) }
