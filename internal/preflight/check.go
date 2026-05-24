package preflight

import (
	"os/exec"
	"runtime"
	"strings"
)

// CheckStatus 检查状态
type CheckStatus int

const (
	StatusPending CheckStatus = iota
	StatusRunning
	StatusOK
	StatusMissing
	StatusInstalling
	StatusInstalled
	StatusFailed
)

func (s CheckStatus) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusRunning:
		return "running"
	case StatusOK:
		return "ok"
	case StatusMissing:
		return "missing"
	case StatusInstalling:
		return "installing"
	case StatusInstalled:
		return "installed"
	case StatusFailed:
		return "failed"
	}
	return "unknown"
}

// Check 定义一个环境检查项
type Check struct {
	Name        string // "Go", "Git"
	Binary      string // 可执行文件名
	Description string // 检查说明
	Required    bool   // 是否必须

	// 安装方式（当 Binary 不存在时执行）
	// 返回安装命令字符串，供用户参考；实际安装由 installer 完成
	InstallHint func() string

	Status CheckStatus
	Error  error
}

// DefaultChecks 返回 Pulse 需要的环境检查列表
// 根据你的实际需求修改
func DefaultChecks() []Check {
	return []Check{
		{
			Name:        "Git",
			Binary:      "git",
			Description: "Version control (used by agent tools)",
			Required:    true,
			InstallHint: func() string {
				switch runtime.GOOS {
				case "darwin":
					return "xcode-select --install"
				case "linux":
					return "sudo apt install git  # or: sudo dnf install git"
				case "windows":
					return "winget install Git.Git"
				}
				return "https://git-scm.com/downloads"
			},
		},
		{
			Name:        "Go",
			Binary:      "go",
			Description: "Go compiler (needed for tool compilation)",
			Required:    true,
			InstallHint: func() string {
				return "https://go.dev/dl/"
			},
		},
	}
}

// CheckBinary 检查某个二进制是否存在于 PATH
func CheckBinary(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// GetVersion 获取某个命令的版本信息（取第一行）
func GetVersion(binary string) string {
	cmd := exec.Command(binary, "--version")
	out, err := cmd.Output()
	if err != nil {
		// 有些工具用 -v 或 -V
		cmd = exec.Command(binary, "-v")
		out, err = cmd.Output()
		if err != nil {
			return ""
		}
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) > 0 {
		v := strings.TrimSpace(lines[0])
		if len(v) > 60 {
			v = v[:57] + "..."
		}
		return v
	}
	return ""
}

// QuickCheck 静默快速检查，不显示 UI，不到 1ms 完成。
// 返回缺失的依赖名列表，空则全部 OK。
func QuickCheck() []string {
	// 先检查缓存是否有效
	if cacheValid() {
		c := loadCache()
		if c != nil {
			var missing []string
			for _, entry := range c.Entries {
				if !entry.OK {
					missing = append(missing, entry.Binary)
				}
			}
			return missing
		}
	}

	// 缓存无效或不存在，执行实际检查
	checks := DefaultChecks()
	var missing []string

	for _, c := range checks {
		if !CheckBinary(c.Binary) {
			missing = append(missing, c.Name)
		}
	}

	// 全部 OK 则更新缓存（让下次也走快检）
	if len(missing) == 0 {
		saveCache(checks)
	}

	return missing
}
