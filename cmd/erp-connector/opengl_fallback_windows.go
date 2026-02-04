//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
)

var openGLFallbackOnce sync.Once

func handleOpenGLFailure() {
	openGLFallbackOnce.Do(func() {
		msg := "OpenGL is not available on this machine (likely Microsoft Hyper-V Video or Basic Display Adapter).\n" +
			"The GUI cannot start. A console window will open with headless configuration options."
		_, _ = windows.MessageBox(0, windows.StringToUTF16Ptr(msg), windows.StringToUTF16Ptr("ERP Connector"), windows.MB_ICONERROR)
		_ = launchHeadlessConsole()
		os.Exit(1)
	})
}

func launchHeadlessConsole() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe = filepath.Clean(exe)
	cmdline := fmt.Sprintf("\"%s\" --headless --show", exe)
	cmd := exec.Command("cmd.exe", "/k", cmdline)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_CONSOLE,
	}
	return cmd.Start()
}
