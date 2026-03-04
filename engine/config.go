package engine

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// Config 游戏配置
type Config struct {
	TextSpeed        float64 `json:"text_speed"`
	AutoPlaySpeed    float64 `json:"auto_play_speed"`
	FontSize         float64 `json:"font_size"`
	FontPath         string  `json:"font_path"`
	SkipMode         string  `json:"skip_mode"`
	SkipSpeed        float64 `json:"skip_speed"`
	ChoiceUnsetAuto  bool    `json:"choice_unset_auto"`
	ChoiceUnsetSkip  bool    `json:"choice_unset_skip"`
	MouseCursorHide  float64 `json:"mouse_cursor_hide"`
	Fullscreen       bool    `json:"fullscreen"`
	WindowW          int     `json:"window_w"`
	WindowH          int     `json:"window_h"`
	MasterVolume     float64 `json:"master_volume"`
	BGMVolume        float64 `json:"bgm_volume"`
	SEVolume         float64 `json:"se_volume"`
	VoiceVolume      float64 `json:"voice_volume"`
	VoiceStopOnClick bool    `json:"voice_stop_on_click"`
	LayerCount       int     `json:"layer_count"`
	SavePath         string  `json:"save_path"`
	Debug            bool    `json:"debug"`

	savePath string
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		TextSpeed:        30.0,
		AutoPlaySpeed:    3.0,
		FontSize:         24,
		FontPath:         "assets/fonts/default.ttf",
		SkipMode:         "read",
		SkipSpeed:        0.05,
		ChoiceUnsetAuto:  true,
		ChoiceUnsetSkip:  true,
		MouseCursorHide:  20.0,
		Fullscreen:       false,
		WindowW:          1280,
		WindowH:          720,
		MasterVolume:     1.0,
		BGMVolume:        0.8,
		SEVolume:         1.0,
		VoiceVolume:      1.0,
		VoiceStopOnClick: false,
		LayerCount:       20,
		SavePath:         "save",
		Debug:            false,
	}
}

// NewConfig 从文件加载配置（修正版：确保目录存在并正确读取）
func NewConfig(path string, fs *FileSystem) (*Config, error) {
	cfg := DefaultConfig()
	cfg.savePath = path

	// 确保save目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("[Config] Cannot create dir %s: %v", dir, err)
	}

	// 读取配置文件
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("[Config] %s not found, using defaults", path)
			// 首次运行：保存默认配置
			if saveErr := cfg.Save(); saveErr != nil {
				log.Printf("[Config] Cannot save default config: %v", saveErr)
			}
		} else {
			log.Printf("[Config] Read error %s: %v", path, err)
		}
		return cfg, nil
	}

	// 解析JSON到临时结构
	loaded := DefaultConfig()
	if err := json.Unmarshal(data, loaded); err != nil {
		log.Printf("[Config] Parse error %s: %v, using defaults", path, err)
		return cfg, nil
	}

	loaded.savePath = path
	log.Printf("[Config] Loaded from %s", path)
	return loaded, nil
}

// Save 保存配置
func (c *Config) Save() error {
	if c.savePath == "" {
		c.savePath = "save/config.json"
	}
	if err := os.MkdirAll(filepath.Dir(c.savePath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(c.savePath, data, 0644); err != nil {
		return err
	}
	log.Printf("[Config] Saved to %s", c.savePath)
	return nil
}
