//go:build !windows

package main

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}

func shellCommand(shell, line string) *exec.Cmd { return exec.Command("sh", "-c", line) }

func killTree(cmd *exec.Cmd) { cmd.Process.Kill() }

func defaultShell() string   { return "sh" }
func shellChoices() []string { return []string{"sh"} }
