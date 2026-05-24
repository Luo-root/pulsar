package preflight

import (
	"fmt"
	"os/exec"
	"runtime"
)

// autoInstallBinary 尝试自动安装缺失的二进制
// 按平台选择安装方式，失败返回 error
func autoInstallBinary(check Check) error {
	switch runtime.GOOS {
	case "darwin":
		return installDarwin(check)
	case "linux":
		return installLinux(check)
	case "windows":
		return installWindows(check)
	}
	return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
}

func installDarwin(check Check) error {
	// 优先 brew
	if CheckBinary("brew") {
		return runCmd("brew", "install", brewFormula(check.Binary))
	}
	return fmt.Errorf("install Homebrew first: https://brew.sh")
}

func installLinux(check Check) error {
	// 优先 apt（Ubuntu/Debian），降级到 dnf
	if CheckBinary("apt") {
		return runCmd("sudo", "apt", "install", "-y", aptPackage(check.Binary))
	}
	if CheckBinary("dnf") {
		return runCmd("sudo", "dnf", "install", "-y", dnfPackage(check.Binary))
	}
	if CheckBinary("pacman") {
		return runCmd("sudo", "pacman", "-S", "--noconfirm", check.Binary)
	}
	return fmt.Errorf("no supported package manager found")
}

func installWindows(check Check) error {
	// winget 优先
	if CheckBinary("winget") {
		return runCmd("winget", "install", wingetPackage(check.Binary))
	}
	return fmt.Errorf("install winget or use: %s", check.InstallHint())
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}

// ── Package name mappings ──────────────────────────────────────────────
// 二进制名到包管理器包名的映射

func brewFormula(binary string) string {
	switch binary {
	case "rg":
		return "ripgrep"
	}
	return binary
}

func aptPackage(binary string) string {
	switch binary {
	case "rg":
		return "ripgrep"
	case "go":
		return "golang-go"
	}
	return binary
}

func dnfPackage(binary string) string {
	switch binary {
	case "rg":
		return "ripgrep"
	case "go":
		return "golang"
	}
	return binary
}

func wingetPackage(binary string) string {
	switch binary {
	case "git":
		return "Git.Git"
	case "rg":
		return "BurntSushi.ripgrep.MSVC"
	case "go":
		return "GoLang.Go"
	}
	return binary
}
