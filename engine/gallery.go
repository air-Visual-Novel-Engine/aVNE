package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// CGEntry CG条目
type CGEntry struct {
	Folder   string   `json:"folder"`
	Files    []string `json:"files"`
	Unlocked bool     `json:"unlocked"`
}

// EventEntry 事件条目
type EventEntry struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ScriptFile string `json:"script_file"`
	Cursor     int    `json:"cursor"`
	Unlocked   bool   `json:"unlocked"`
}

// GalleryData 鉴赏数据
type GalleryData struct {
	CGs    map[string]*CGEntry    `json:"cgs"`
	Events map[string]*EventEntry `json:"events"`
}

// GallerySystem 鉴赏系统
type GallerySystem struct {
	savePath string
	data     GalleryData
}

// NewGallerySystem 创建鉴赏系统
func NewGallerySystem(fs *FileSystem, savePath string) (*GallerySystem, error) {
	gs := &GallerySystem{
		savePath: savePath,
		data: GalleryData{
			CGs:    make(map[string]*CGEntry),
			Events: make(map[string]*EventEntry),
		},
	}

	// 从文件加载
	if b, err := os.ReadFile(savePath); err == nil {
		json.Unmarshal(b, &gs.data)
	}

	if gs.data.CGs == nil {
		gs.data.CGs = make(map[string]*CGEntry)
	}
	if gs.data.Events == nil {
		gs.data.Events = make(map[string]*EventEntry)
	}

	return gs, nil
}

// RecordCG 记录CG
func (gs *GallerySystem) RecordCG(file string) {
	folder := filepath.Dir(file)
	entry, ok := gs.data.CGs[folder]
	if !ok {
		entry = &CGEntry{Folder: folder}
		gs.data.CGs[folder] = entry
	}

	// 检查是否已记录
	for _, f := range entry.Files {
		if f == file {
			return
		}
	}
	entry.Files = append(entry.Files, file)
	entry.Unlocked = true
	gs.Save()
}

// RecordCGWithFolder 记录CG（指定文件夹）
func (gs *GallerySystem) RecordCGWithFolder(file, folder string) {
	if folder == "" {
		folder = filepath.Dir(file)
	}
	entry, ok := gs.data.CGs[folder]
	if !ok {
		entry = &CGEntry{Folder: folder}
		gs.data.CGs[folder] = entry
	}

	for _, f := range entry.Files {
		if f == file {
			return
		}
	}
	entry.Files = append(entry.Files, file)
	entry.Unlocked = true
	gs.Save()
}

// RecordEvent 记录事件
func (gs *GallerySystem) RecordEvent(scriptFile string, cursor int) {
	id := fmt.Sprintf("%s:%d", scriptFile, cursor)
	if _, ok := gs.data.Events[id]; ok {
		gs.data.Events[id].Unlocked = true
		gs.Save()
		return
	}
}

// RegisterEvent 注册事件（在脚本中标记）
func (gs *GallerySystem) RegisterEvent(id, name, scriptFile string, cursor int) {
	if _, ok := gs.data.Events[id]; !ok {
		gs.data.Events[id] = &EventEntry{
			ID:         id,
			Name:       name,
			ScriptFile: scriptFile,
			Cursor:     cursor,
			Unlocked:   false,
		}
	}
}

// UnlockEvent 解锁事件
func (gs *GallerySystem) UnlockEvent(id string) {
	if e, ok := gs.data.Events[id]; ok {
		e.Unlocked = true
		gs.Save()
	}
}

// GetUnlockedCGs 获取已解锁CG列表
func (gs *GallerySystem) GetUnlockedCGs() []*CGEntry {
	var result []*CGEntry
	for _, entry := range gs.data.CGs {
		if entry.Unlocked {
			result = append(result, entry)
		}
	}
	return result
}

// GetUnlockedEvents 获取已解锁事件列表
func (gs *GallerySystem) GetUnlockedEvents() []*EventEntry {
	var result []*EventEntry
	for _, entry := range gs.data.Events {
		if entry.Unlocked {
			result = append(result, entry)
		}
	}
	return result
}

// GetAllCGs 获取所有CG（包括未解锁）
func (gs *GallerySystem) GetAllCGs() []*CGEntry {
	var result []*CGEntry
	for _, entry := range gs.data.CGs {
		result = append(result, entry)
	}
	return result
}

// GetAllEvents 获取所有事件
func (gs *GallerySystem) GetAllEvents() []*EventEntry {
	var result []*EventEntry
	for _, entry := range gs.data.Events {
		result = append(result, entry)
	}
	return result
}

// Save 保存鉴赏数据
func (gs *GallerySystem) Save() {
	if gs.savePath == "" {
		return
	}
	os.MkdirAll(filepath.Dir(gs.savePath), 0755)
	b, err := json.MarshalIndent(gs.data, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(gs.savePath, b, 0644)
}
