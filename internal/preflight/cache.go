package preflight

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const cacheTTL = 24 * time.Hour // 缓存有效期

type cacheEntry struct {
	Binary  string    `json:"binary"`
	OK      bool      `json:"ok"`
	Version string    `json:"version"`
	Checked time.Time `json:"checked"`
}

type cacheData struct {
	Version string       `json:"version"`
	Entries []cacheEntry `json:"entries"`
}

func cachePath() string {
	dir, _ := os.UserConfigDir()
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, ".pulse-tui", ".preflight")
}

// loadCache 读取缓存，不存在或损坏返回 nil
func loadCache() *cacheData {
	data, err := os.ReadFile(cachePath())
	if err != nil {
		return nil
	}
	var c cacheData
	if json.Unmarshal(data, &c) != nil {
		return nil
	}
	return &c
}

func saveCache(checks []Check) {
	c := cacheData{Version: "1"}
	for _, ch := range checks {
		c.Entries = append(c.Entries, cacheEntry{
			Binary:  ch.Binary,
			OK:      ch.Status == StatusOK || ch.Status == StatusInstalled,
			Version: GetVersion(ch.Binary),
			Checked: time.Now(),
		})
	}
	path := cachePath()
	os.MkdirAll(filepath.Dir(path), 0o755)
	b, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(path, b, 0o644)
}

// cacheValid 检查缓存是否还在有效期内
func cacheValid() bool {
	c := loadCache()
	if c == nil {
		return false
	}
	if len(c.Entries) == 0 {
		return false
	}
	// 任意一条过期就全部重检
	for _, e := range c.Entries {
		if time.Since(e.Checked) > cacheTTL {
			return false
		}
	}
	return true
}
