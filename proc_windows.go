//go:build windows

package main

import (
	"os/exec"
	"strconv"
	"syscall"
)

func hideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000
}

func shellCommand(shell, line string) *exec.Cmd {
	var cmd *exec.Cmd
	if shell == "powershell" {
		cmd = exec.Command("powershell.exe")
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -Command " + line}
	} else {
		cmd = exec.Command("cmd.exe")
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "cmd.exe /S /C \"" + line + "\""}
	}
	return cmd
}

func killTree(cmd *exec.Cmd) {
	k := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	hideWindow(k)
	k.Run()
}

func defaultShell() string   { return "cmd" }
func shellChoices() []string { return []string{"cmd", "powershell"} }
