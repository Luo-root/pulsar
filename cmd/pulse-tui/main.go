package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"pulse-tui/internal/bootloader"
	"pulse-tui/internal/preflight"
	"pulse-tui/internal/view"
	"pulse-tui/internal/worker"

	tea "charm.land/bubbletea/v2"
)

func LoadConfig(path string) *bootloader.Config {
	config, err := bootloader.LoadConfig(path)
	if !errors.Is(err, bootloader.ErrConfigNotFound) && err != nil {
		panic("出了一点小问题呢：" + err.Error())
	}
	return config
}

func main() {
	// ── 解析子命令 ──
	autoInstall := hasFlag("--install-deps")
	forceCheck := hasFlag("--check")

	switch {
	case autoInstall:
		// 强制全量检查 + 自动安装
		if runPreflight(true) {
			return
		}

	case forceCheck:
		// 强制全量检查（不含自动安装）
		if runPreflight(false) {
			return
		}

	default:
		// ── 默认：静默快检 ──
		// 不到 1ms，用户无感知
		missing := preflight.QuickCheck()
		if len(missing) > 0 {
			// 有缺失，弹出全量检查动画
			fmt.Printf("⚠ Missing dependencies: %v\n", missing)
			if runPreflight(false) {
				return
			}
		}
		// 全部 OK，直接跳过，不显示任何检查 UI
	}

	path, err := bootloader.GetDefaultConfigPath()
	if err != nil {
		panic("出了一点小问题呢：" + err.Error())
	}
	config := LoadConfig(path)
	if config == nil {
		bootloader.BootLoader()
	}
	config = LoadConfig(path)
	ctx := context.Background()
	manager, err := worker.NewManager(ctx, config)
	if err != nil {
		panic(fmt.Sprintf("Worker 初始化失败: %v", err))
	}
	view.UI(ctx, manager)
}

func hasFlag(flag string) bool {
	for _, arg := range os.Args[1:] {
		if arg == flag {
			return true
		}
	}
	return false
}

func runPreflight(autoInstall bool) (shouldExit bool) {
	m := preflight.New(autoInstall)
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Preflight error: %v\n", err)
		os.Exit(1)
	}
	final := result.(*preflight.Model)
	if final.Failed() {
		fmt.Fprintln(os.Stderr, "\nRerun with --install-deps to auto-install missing dependencies.")
		os.Exit(1)
	}
	return false
}
