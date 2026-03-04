package engine

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	MaxSaveSlots  = 99
	MaxBacklogLen = 30
)

// SaveData 存档数据
type SaveData struct {
	Slot       int               `json:"slot"`
	Date       string            `json:"date"`
	ScriptFile string            `json:"script_file"`
	Cursor     int               `json:"cursor"`
	Vars       map[string]int    `json:"vars"`
	StrVars    map[string]string `json:"str_vars"`
	Affinity   map[string]int    `json:"affinity"`
	Backlog    []BacklogEntry    `json:"backlog"`
	Thumbnail  string            `json:"thumbnail"` // base64 PNG
	LayerState []LayerSaveState  `json:"layer_state"`
	BGMPath    string            `json:"bgm_path"`
	Config     *Config           `json:"config,omitempty"`
}

// BacklogEntry backlog条目
type BacklogEntry struct {
	Speaker   string `json:"speaker"`
	Text      string `json:"text"`
	VoicePath string `json:"voice_path"`
}

// LayerSaveState 层状态存档
type LayerSaveState struct {
	Index   int     `json:"index"`
	ImgPath string  `json:"img_path"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	ScaleX  float64 `json:"scale_x"`
	ScaleY  float64 `json:"scale_y"`
	Alpha   float64 `json:"alpha"`
	Visible bool    `json:"visible"`
}

// SaveSystem 存档系统
type SaveSystem struct {
	savePath  string
	quickSave *SaveData
}

// NewSaveSystem 创建存档系统
func NewSaveSystem(savePath string, eng *Engine) (*SaveSystem, error) {
	ss := &SaveSystem{
		savePath: savePath,
	}
	if err := os.MkdirAll(savePath, 0755); err != nil {
		return nil, err
	}
	return ss, nil
}

// Save 保存到指定槽位
func (ss *SaveSystem) Save(slot int, eng *Engine) error {
	data := ss.captureState(slot, eng)

	path := filepath.Join(ss.savePath, fmt.Sprintf("save_%03d.json", slot))
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

// Load 从指定槽位读档
func (ss *SaveSystem) Load(slot int, eng *Engine) error {
	path := filepath.Join(ss.savePath, fmt.Sprintf("save_%03d.json", slot))
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var data SaveData
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	return ss.restoreState(&data, eng)
}

// QuickSave 快速存档
func (ss *SaveSystem) QuickSave(eng *Engine) {
	data := ss.captureState(0, eng)
	ss.quickSave = data

	path := filepath.Join(ss.savePath, "quick_save.json")
	b, _ := json.MarshalIndent(data, "", "  ")
	os.WriteFile(path, b, 0644)
}

// QuickLoad 快速读档
func (ss *SaveSystem) QuickLoad(eng *Engine) {
	if ss.quickSave != nil {
		ss.restoreState(ss.quickSave, eng)
		return
	}
	path := filepath.Join(ss.savePath, "quick_save.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var data SaveData
	if err := json.Unmarshal(b, &data); err != nil {
		return
	}
	ss.restoreState(&data, eng)
}

// GetSaveData 获取存档数据（不恢复，只读取）
func (ss *SaveSystem) GetSaveData(slot int) (*SaveData, error) {
	path := filepath.Join(ss.savePath, fmt.Sprintf("save_%03d.json", slot))
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data SaveData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// GetQuickSaveData 获取快速存档数据
func (ss *SaveSystem) GetQuickSaveData() *SaveData {
	if ss.quickSave != nil {
		return ss.quickSave
	}
	path := filepath.Join(ss.savePath, "quick_save.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var data SaveData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil
	}
	return &data
}

func (ss *SaveSystem) captureState(slot int, eng *Engine) *SaveData {
	data := &SaveData{
		Slot:       slot,
		Date:       time.Now().Format("2006-01-02 15:04:05"),
		ScriptFile: eng.script.currentFile,
		Cursor:     eng.script.GetCursor(),
		Vars:       make(map[string]int),
		StrVars:    make(map[string]string),
		Affinity:   make(map[string]int),
	}

	// 复制变量
	for k, v := range eng.script.vars {
		data.Vars[k] = v
	}
	for k, v := range eng.script.strVars {
		data.StrVars[k] = v
	}
	for k, v := range eng.script.affinity {
		data.Affinity[k] = v
	}

	// 保存backlog（最近30条）
	history := eng.textSys.GetHistory()
	start := 0
	if len(history) > MaxBacklogLen {
		start = len(history) - MaxBacklogLen
	}
	for _, h := range history[start:] {
		data.Backlog = append(data.Backlog, BacklogEntry{
			Speaker:   h.SpeakerName,
			Text:      h.AllText,
			VoicePath: h.VoicePath,
		})
	}

	// 保存层状态
	for i, layer := range eng.layers.layers {
		if layer.Visible {
			ls := LayerSaveState{
				Index:   i,
				X:       layer.X,
				Y:       layer.Y,
				ScaleX:  layer.ScaleX,
				ScaleY:  layer.ScaleY,
				Alpha:   layer.Alpha,
				Visible: layer.Visible,
			}
			data.LayerState = append(data.LayerState, ls)
		}
	}

	// 保存BGM
	if eng.audio.bgm != nil {
		data.BGMPath = eng.audio.bgm.path
	}

	// 截图缩略图
	thumb := eng.GetScreenshot()
	data.Thumbnail = encodeImageToBase64(thumb)

	return data
}

func (ss *SaveSystem) restoreState(data *SaveData, eng *Engine) error {
	// 恢复变量
	eng.script.vars = make(map[string]int)
	eng.script.strVars = make(map[string]string)
	eng.script.affinity = make(map[string]int)

	for k, v := range data.Vars {
		eng.script.vars[k] = v
	}
	for k, v := range data.StrVars {
		eng.script.strVars[k] = v
	}
	for k, v := range data.Affinity {
		eng.script.affinity[k] = v
	}

	// 恢复层状态
	for _, ls := range data.LayerState {
		layer := eng.layers.GetLayer(ls.Index)
		if layer != nil {
			layer.X = ls.X
			layer.Y = ls.Y
			layer.ScaleX = ls.ScaleX
			layer.ScaleY = ls.ScaleY
			layer.Alpha = ls.Alpha
			layer.Visible = ls.Visible
		}
	}

	// 恢复BGM
	if data.BGMPath != "" {
		eng.audio.PlayBGM(data.BGMPath, true)
	}

	// 加载脚本并跳转
	if data.ScriptFile != "" {
		if err := eng.script.LoadScript(data.ScriptFile); err != nil {
			return err
		}
		eng.script.SetCursor(data.Cursor)
		eng.script.runUntilDisplay()
	}

	eng.SetState(StateGame)
	return nil
}

func encodeImageToBase64(img *ebiten.Image) string {
	if img == nil {
		return ""
	}
	rgba := img.Bounds()
	_ = rgba

	var buf bytes.Buffer
	pngImg := img
	// 将ebiten图片转为image.NRGBA
	nrgba := image.NewNRGBA(img.Bounds())
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			nrgba.Set(x, y, pngImg.At(x, y))
		}
	}
	if err := png.Encode(&buf, nrgba); err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// DecodeBase64Image 解码base64图片
func DecodeBase64Image(b64 string) (*ebiten.Image, error) {
	if b64 == "" {
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return ebiten.NewImageFromImage(img), nil
}
