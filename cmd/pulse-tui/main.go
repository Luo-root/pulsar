package main

import (
	"context"
	"errors"
	"fmt"
	"pulse-tui/internal/bootloader"
	"pulse-tui/internal/view"
	"pulse-tui/internal/worker"
)

func LoadConfig(path string) *bootloader.Config {
	config, err := bootloader.LoadConfig(path)
	if !errors.Is(err, bootloader.ErrConfigNotFound) && err != nil {
		panic("出了一点小问题呢：" + err.Error())
	}
	return config
}

func main() {
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
	fmt.Println("✨ Pulse-TUI 启动成功！正在进入聊天界面...")
	view.UI(ctx, manager)
}
