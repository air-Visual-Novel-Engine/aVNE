package engine

import (
	"fmt"
	"log"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	lua "github.com/yuin/gopher-lua"
	"image/color"
)

// LuaBridge Lua桥接
type LuaBridge struct {
	eng *Engine
	L   *lua.LState
}

// NewLuaBridge 创建Lua桥接
func NewLuaBridge(eng *Engine) (*LuaBridge, error) {
	lb := &LuaBridge{
		eng: eng,
		L:   lua.NewState(),
	}
	lb.registerFunctions()
	return lb, nil
}

// LoadFile 加载Lua文件
func (lb *LuaBridge) LoadFile(path string) error {
	data, err := lb.eng.fs.ReadFile(path)
	if err != nil {
		return err
	}
	return lb.L.DoString(string(data))
}

// CallFunction 调用Lua函数
func (lb *LuaBridge) CallFunction(name string, args ...interface{}) {
	fn := lb.L.GetGlobal(name)
	if fn == lua.LNil {
		return
	}
	luaArgs := make([]lua.LValue, len(args))
	for i, arg := range args {
		luaArgs[i] = lb.goToLua(arg)
	}
	if err := lb.L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    0,
		Protect: true,
	}, luaArgs...); err != nil {
		log.Printf("Lua error in %s: %v", name, err)
	}
}

// CallDraw 调用Lua绘制函数
func (lb *LuaBridge) CallDraw(name string, screen *ebiten.Image) {
	fn := lb.L.GetGlobal(name)
	if fn == lua.LNil {
		return
	}
	ud := lb.L.NewUserData()
	ud.Value = screen
	if err := lb.L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    0,
		Protect: true,
	}, ud); err != nil {
		log.Printf("Lua draw error in %s: %v", name, err)
	}
}

func (lb *LuaBridge) goToLua(v interface{}) lua.LValue {
	if v == nil {
		return lua.LNil
	}
	switch val := v.(type) {
	case bool:
		return lua.LBool(val)
	case int:
		return lua.LNumber(val)
	case int64:
		return lua.LNumber(val)
	case float64:
		return lua.LNumber(val)
	case string:
		return lua.LString(val)
	case []interface{}:
		tbl := lb.L.NewTable()
		for i, item := range val {
			tbl.RawSetInt(i+1, lb.goToLua(item))
		}
		return tbl
	case map[string]interface{}:
		tbl := lb.L.NewTable()
		for k, item := range val {
			tbl.RawSetString(k, lb.goToLua(item))
		}
		return tbl
	}
	return lua.LNil
}

var (
	clipImages = map[string]*ebiten.Image{}
)

func (lb *LuaBridge) registerFunctions() {
	L := lb.L

	// 强制重置并返回标题界面（如游戏结束后用）
	L.SetGlobal("engine_reset_title", L.NewFunction(func(L *lua.LState) int {
		lb.eng.ForceResetTitle()
		return 0
	}))

	// 慢速淡黑 + 完成后执行Lua函数
	L.SetGlobal("engine_start_transition_with_callback", L.NewFunction(func(L *lua.LState) int {
		to := GameState(int(L.CheckNumber(1)))
		duration := float64(L.OptNumber(2, 5.0))
		callbackName := L.OptString(3, "")
		speed := 1.0 / duration

		lb.eng.transition.active = true
		lb.eng.transitionTo = to
		lb.eng.transition.speed = speed
		lb.eng.transition.alpha = 0
		lb.eng.transition.fadeIn = false

		if callbackName != "" {
			lb.eng.transition.callback = func() {
				lb.CallFunction(callbackName)
			}
		}
		return 0
	}))

	L.SetGlobal("clip_begin", L.NewFunction(func(L *lua.LState) int {
		w := int(L.CheckNumber(1))
		h := int(L.CheckNumber(2))
		if w <= 0 || h <= 0 {
			L.Push(lua.LNil)
			return 1
		}

		key := fmt.Sprintf("%dx%d", w, h)
		img, ok := clipImages[key]
		if !ok || img.Bounds().Dx() != w || img.Bounds().Dy() != h {
			img = ebiten.NewImage(w, h)
			clipImages[key] = img
		}
		img.Clear()

		ud := L.NewUserData()
		ud.Value = img
		L.Push(ud)
		return 1
	}))

	L.SetGlobal("clip_end", L.NewFunction(func(L *lua.LState) int {
		screen := lb.getScreen(L, 1)
		ud, ok := L.Get(2).(*lua.LUserData)
		if !ok || screen == nil {
			return 0
		}
		clipImg, ok := ud.Value.(*ebiten.Image)
		if !ok || clipImg == nil {
			return 0
		}
		dx := float64(L.CheckNumber(3))
		dy := float64(L.CheckNumber(4))
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(dx, dy)
		screen.DrawImage(clipImg, op)
		return 0
	}))
	// 鼠标左键持续按住
	L.SetGlobal("input_left_held", L.NewFunction(func(L *lua.LState) int {
		held := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
		L.Push(lua.LBool(held))
		return 1
	}))

	// 滚轮增量（向上为正）
	L.SetGlobal("input_scroll_delta", L.NewFunction(func(L *lua.LState) int {
		_, dy := ebiten.Wheel()
		L.Push(lua.LNumber(dy))
		return 1
	}))

	// Ctrl键是否按住
	L.SetGlobal("input_ctrl_held", L.NewFunction(func(L *lua.LState) int {
		held := ebiten.IsKeyPressed(ebiten.KeyControlLeft) ||
			ebiten.IsKeyPressed(ebiten.KeyControlRight)
		L.Push(lua.LBool(held))
		return 1
	}))

	// 游戏是否正在快进（F8或Ctrl）
	L.SetGlobal("game_is_skipping", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LBool(lb.eng.skipMode || lb.eng.ctrlSkip))
		return 1
	}))
	// ===== 引擎控制 =====
	L.SetGlobal("engine_set_state", L.NewFunction(func(L *lua.LState) int {
		state := int(L.CheckNumber(1))
		lb.eng.SetState(GameState(state))
		return 0
	}))

	L.SetGlobal("engine_start_transition", L.NewFunction(func(L *lua.LState) int {
		toState := int(L.CheckNumber(1))
		fadeOut := L.OptBool(2, false)
		lb.eng.StartTransition(GameState(toState), fadeOut)
		return 0
	}))

	L.SetGlobal("engine_get_state", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(lb.eng.state))
		return 1
	}))

	L.SetGlobal("engine_get_prev_state", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(lb.eng.prevState))
		return 1
	}))

	// 修复：使用 eng.quitting 字段
	L.SetGlobal("engine_quit", L.NewFunction(func(L *lua.LState) int {
		lb.eng.quitting = true
		return 0
	}))

	// ===== 图片绘制 =====
	L.SetGlobal("draw_image", L.NewFunction(func(L *lua.LState) int {
		screen := lb.getScreen(L, 1)
		path := L.CheckString(2)
		x := float64(L.OptNumber(3, 0))
		y := float64(L.OptNumber(4, 0))
		alpha := float64(L.OptNumber(5, 1.0))
		scaleX := float64(L.OptNumber(6, 1.0))
		scaleY := float64(L.OptNumber(7, 1.0))
		if screen == nil {
			return 0
		}
		img, err := lb.eng.cache.GetImage(path)
		if err != nil {
			return 0
		}
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(scaleX, scaleY)
		op.GeoM.Translate(x, y)
		op.ColorScale.ScaleAlpha(float32(alpha))
		screen.DrawImage(img, op)
		return 0
	}))

	L.SetGlobal("draw_image_centered", L.NewFunction(func(L *lua.LState) int {
		screen := lb.getScreen(L, 1)
		path := L.CheckString(2)
		x := float64(L.CheckNumber(3))
		y := float64(L.CheckNumber(4))
		alpha := float64(L.OptNumber(5, 1.0))
		scaleX := float64(L.OptNumber(6, 1.0))
		scaleY := float64(L.OptNumber(7, 1.0))
		if screen == nil {
			return 0
		}
		img, err := lb.eng.cache.GetImage(path)
		if err != nil {
			return 0
		}
		w := float64(img.Bounds().Dx())
		h := float64(img.Bounds().Dy())
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(scaleX, scaleY)
		op.GeoM.Translate(x-w*scaleX/2, y-h*scaleY/2)
		op.ColorScale.ScaleAlpha(float32(alpha))
		screen.DrawImage(img, op)
		return 0
	}))

	L.SetGlobal("get_image_size", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		img, err := lb.eng.cache.GetImage(path)
		if err != nil {
			L.Push(lua.LNumber(0))
			L.Push(lua.LNumber(0))
			return 2
		}
		L.Push(lua.LNumber(img.Bounds().Dx()))
		L.Push(lua.LNumber(img.Bounds().Dy()))
		return 2
	}))

	// ===== 文字绘制 =====
	L.SetGlobal("draw_text", L.NewFunction(func(L *lua.LState) int {
		screen := lb.getScreen(L, 1)
		txt := L.CheckString(2)
		x := float64(L.CheckNumber(3))
		y := float64(L.CheckNumber(4))
		size := float64(L.OptNumber(5, 24))
		r := uint8(L.OptNumber(6, 255))
		g := uint8(L.OptNumber(7, 255))
		b := uint8(L.OptNumber(8, 255))
		a := float32(L.OptNumber(9, 1.0))
		if screen == nil {
			return 0
		}
		style := DefaultTextStyle()
		style.FontSize = size
		style.Color = color.RGBA{R: r, G: g, B: b, A: 255}
		lb.eng.textSys.drawTextWithOutline(screen, txt, x, y, style, a)
		return 0
	}))

	L.SetGlobal("draw_rect", L.NewFunction(func(L *lua.LState) int {
		screen := lb.getScreen(L, 1)
		x := float64(L.CheckNumber(2))
		y := float64(L.CheckNumber(3))
		w := float64(L.CheckNumber(4))
		h := float64(L.CheckNumber(5))
		r := uint8(L.OptNumber(6, 0))
		g := uint8(L.OptNumber(7, 0))
		b := uint8(L.OptNumber(8, 0))
		a := uint8(L.OptNumber(9, 200))
		if screen == nil {
			return 0
		}
		ebitenutil.DrawRect(screen, x, y, w, h, color.RGBA{R: r, G: g, B: b, A: a})
		return 0
	}))

	// ===== 输入 =====
	L.SetGlobal("input_mouse_pos", L.NewFunction(func(L *lua.LState) int {
		x, y := lb.eng.input.MousePosition()
		L.Push(lua.LNumber(x))
		L.Push(lua.LNumber(y))
		return 2
	}))

	L.SetGlobal("input_left_click", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LBool(lb.eng.input.IsLeftClickJustPressed()))
		return 1
	}))

	L.SetGlobal("input_right_click", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LBool(lb.eng.input.IsRightClickJustPressed()))
		return 1
	}))

	L.SetGlobal("input_mouse_over", L.NewFunction(func(L *lua.LState) int {
		x := float64(L.CheckNumber(1))
		y := float64(L.CheckNumber(2))
		w := float64(L.CheckNumber(3))
		h := float64(L.CheckNumber(4))
		L.Push(lua.LBool(lb.eng.input.IsMouseOverF(x, y, w, h)))
		return 1
	}))

	L.SetGlobal("input_key_just_pressed", L.NewFunction(func(L *lua.LState) int {
		keyName := L.CheckString(1)
		key := parseKeyName(keyName)
		L.Push(lua.LBool(lb.eng.input.IsKeyJustPressed(key)))
		return 1
	}))

	// ===== 层管理 =====
	L.SetGlobal("layer_set_image", L.NewFunction(func(L *lua.LState) int {
		idx := int(L.CheckNumber(1))
		path := L.CheckString(2)
		x := float64(L.OptNumber(3, 0))
		y := float64(L.OptNumber(4, 0))
		lb.eng.layers.SetLayerImage(idx, path, x, y)
		return 0
	}))

	L.SetGlobal("layer_clear", L.NewFunction(func(L *lua.LState) int {
		lb.eng.layers.ClearLayer(int(L.CheckNumber(1)))
		return 0
	}))

	L.SetGlobal("layer_fade", L.NewFunction(func(L *lua.LState) int {
		idx := int(L.CheckNumber(1))
		alpha := float64(L.CheckNumber(2))
		duration := float64(L.OptNumber(3, 0.5))
		lb.eng.layers.FadeLayer(idx, alpha, duration, EaseInOut)
		return 0
	}))

	L.SetGlobal("layer_move", L.NewFunction(func(L *lua.LState) int {
		idx := int(L.CheckNumber(1))
		x := float64(L.CheckNumber(2))
		y := float64(L.CheckNumber(3))
		duration := float64(L.OptNumber(4, 0.5))
		lb.eng.layers.MoveLayer(idx, x, y, duration, EaseInOut)
		return 0
	}))

	L.SetGlobal("layer_scale", L.NewFunction(func(L *lua.LState) int {
		idx := int(L.CheckNumber(1))
		sx := float64(L.CheckNumber(2))
		sy := float64(L.OptNumber(3, 0))
		if sy == 0 {
			sy = sx
		}
		duration := float64(L.OptNumber(4, 0.5))
		lb.eng.layers.ScaleLayer(idx, sx, sy, duration, EaseInOut)
		return 0
	}))

	L.SetGlobal("layer_set_alpha", L.NewFunction(func(L *lua.LState) int {
		idx := int(L.CheckNumber(1))
		alpha := float64(L.CheckNumber(2))
		layer := lb.eng.layers.GetLayer(idx)
		if layer != nil {
			layer.Alpha = alpha
		}
		return 0
	}))

	L.SetGlobal("layer_is_animating", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LBool(lb.eng.layers.IsAnimating()))
		return 1
	}))

	// ===== 音频 =====
	L.SetGlobal("audio_play_bgm", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		loop := L.OptBool(2, true)
		lb.eng.audio.PlayBGM(path, loop)
		return 0
	}))

	L.SetGlobal("audio_stop_bgm", L.NewFunction(func(L *lua.LState) int {
		lb.eng.audio.StopBGM()
		return 0
	}))

	L.SetGlobal("audio_play_se", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		loop := L.OptBool(2, false)
		lb.eng.audio.PlaySE(path, loop)
		return 0
	}))

	L.SetGlobal("audio_play_voice", L.NewFunction(func(L *lua.LState) int {
		lb.eng.audio.PlayVoice(L.CheckString(1))
		return 0
	}))

	L.SetGlobal("audio_replay_voice", L.NewFunction(func(L *lua.LState) int {
		lb.eng.audio.ReplayVoice()
		return 0
	}))

	L.SetGlobal("audio_set_bgm_volume", L.NewFunction(func(L *lua.LState) int {
		lb.eng.audio.SetBGMVolume(float64(L.CheckNumber(1)))
		return 0
	}))

	L.SetGlobal("audio_set_se_volume", L.NewFunction(func(L *lua.LState) int {
		lb.eng.audio.SetSEVolume(float64(L.CheckNumber(1)))
		return 0
	}))

	L.SetGlobal("audio_set_voice_volume", L.NewFunction(func(L *lua.LState) int {
		lb.eng.audio.SetVoiceVolume(float64(L.CheckNumber(1)))
		return 0
	}))

	L.SetGlobal("audio_set_master_volume", L.NewFunction(func(L *lua.LState) int {
		lb.eng.audio.SetMasterVolume(float64(L.CheckNumber(1)))
		return 0
	}))

	L.SetGlobal("audio_is_voice_playing", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LBool(lb.eng.audio.IsVoicePlaying()))
		return 1
	}))

	L.SetGlobal("audio_toggle_voice", L.NewFunction(func(L *lua.LState) int {
		lb.eng.audio.ToggleVoice()
		return 0
	}))

	// ===== 配置 =====
	L.SetGlobal("config_get", L.NewFunction(func(L *lua.LState) int {
		L.Push(lb.getConfigValue(L.CheckString(1)))
		return 1
	}))

	L.SetGlobal("config_set", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(1)
		val := L.Get(2)
		lb.setConfigValue(key, val)
		if err := lb.eng.config.Save(); err != nil {
			log.Printf("config save error: %v", err)
		}
		// ★ 关键：设置后立即重新应用到引擎
		lb.eng.ReloadConfig()
		return 0
	}))

	// ===== 存档 =====
	L.SetGlobal("save_game", L.NewFunction(func(L *lua.LState) int {
		slot := int(L.CheckNumber(1))
		if err := lb.eng.saveLoad.Save(slot, lb.eng); err != nil {
			L.Push(lua.LBool(false))
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LBool(true))
		return 1
	}))

	L.SetGlobal("load_game", L.NewFunction(func(L *lua.LState) int {
		slot := int(L.CheckNumber(1))
		if err := lb.eng.saveLoad.Load(slot, lb.eng); err != nil {
			L.Push(lua.LBool(false))
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LBool(true))
		return 1
	}))

	L.SetGlobal("quick_save", L.NewFunction(func(L *lua.LState) int {
		lb.eng.saveLoad.QuickSave(lb.eng)
		return 0
	}))

	L.SetGlobal("quick_load", L.NewFunction(func(L *lua.LState) int {
		lb.eng.saveLoad.QuickLoad(lb.eng)
		return 0
	}))

	L.SetGlobal("get_save_info", L.NewFunction(func(L *lua.LState) int {
		data, err := lb.eng.saveLoad.GetSaveData(int(L.CheckNumber(1)))
		if err != nil {
			L.Push(lua.LNil)
			return 1
		}
		tbl := L.NewTable()
		tbl.RawSetString("date", lua.LString(data.Date))
		tbl.RawSetString("thumbnail", lua.LString(data.Thumbnail))
		tbl.RawSetString("slot", lua.LNumber(data.Slot))
		L.Push(tbl)
		return 1
	}))

	L.SetGlobal("get_quick_save_info", L.NewFunction(func(L *lua.LState) int {
		data := lb.eng.saveLoad.GetQuickSaveData()
		if data == nil {
			L.Push(lua.LNil)
			return 1
		}
		tbl := L.NewTable()
		tbl.RawSetString("date", lua.LString(data.Date))
		tbl.RawSetString("thumbnail", lua.LString(data.Thumbnail))
		L.Push(tbl)
		return 1
	}))

	// ===== 鉴赏 =====
	L.SetGlobal("gallery_get_cgs", L.NewFunction(func(L *lua.LState) int {
		cgs := lb.eng.gallery.GetUnlockedCGs()
		tbl := L.NewTable()
		for i, cg := range cgs {
			cgTbl := L.NewTable()
			cgTbl.RawSetString("folder", lua.LString(cg.Folder))
			ft := L.NewTable()
			for j, f := range cg.Files {
				ft.RawSetInt(j+1, lua.LString(f))
			}
			cgTbl.RawSetString("files", ft)
			tbl.RawSetInt(i+1, cgTbl)
		}
		L.Push(tbl)
		return 1
	}))
	L.SetGlobal("gallery_get_events", L.NewFunction(func(L *lua.LState) int {
		events := lb.eng.gallery.GetUnlockedEvents()
		tbl := L.NewTable()
		for i, ev := range events {
			evTbl := L.NewTable()
			evTbl.RawSetString("id", lua.LString(ev.ID))
			evTbl.RawSetString("name", lua.LString(ev.Name))
			tbl.RawSetInt(i+1, evTbl)
		}
		L.Push(tbl)
		return 1
	}))

	L.SetGlobal("gallery_replay_event", L.NewFunction(func(L *lua.LState) int {
		id := L.CheckString(1)
		for _, ev := range lb.eng.gallery.GetAllEvents() {
			if ev.ID == id && ev.Unlocked {
				lb.eng.script.LoadScript(ev.ScriptFile)
				lb.eng.script.SetCursor(ev.Cursor)
				lb.eng.SetState(StateGame)
				break
			}
		}
		return 0
	}))

	// ===== 脚本控制 =====
	L.SetGlobal("script_start", L.NewFunction(func(L *lua.LState) int {
		path := L.CheckString(1)
		if err := lb.eng.script.LoadScript(path); err != nil {
			log.Printf("script_start error: %v", err)
		}
		lb.eng.SetState(StateGame)
		return 0
	}))

	L.SetGlobal("script_advance", L.NewFunction(func(L *lua.LState) int {
		lb.eng.script.Advance()
		return 0
	}))

	L.SetGlobal("script_select_choice", L.NewFunction(func(L *lua.LState) int {
		idx := int(L.CheckNumber(1)) - 1
		lb.eng.script.SelectChoice(idx)
		return 0
	}))

	L.SetGlobal("script_has_choices", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LBool(lb.eng.script.HasChoices()))
		return 1
	}))

	L.SetGlobal("script_get_choices", L.NewFunction(func(L *lua.LState) int {
		tbl := L.NewTable()
		for i, c := range lb.eng.script.choices {
			ct := L.NewTable()
			ct.RawSetString("text", lua.LString(c.Text))
			ct.RawSetString("target", lua.LString(c.Target))
			tbl.RawSetInt(i+1, ct)
		}
		L.Push(tbl)
		return 1
	}))

	L.SetGlobal("script_get_var", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(lb.eng.script.GetVar(L.CheckString(1))))
		return 1
	}))

	L.SetGlobal("script_get_affinity", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(lb.eng.script.GetAffinity(L.CheckString(1))))
		return 1
	}))

	// ===== Backlog =====
	L.SetGlobal("backlog_get", L.NewFunction(func(L *lua.LState) int {
		history := lb.eng.textSys.GetHistory()
		tbl := L.NewTable()
		start := 0
		if len(history) > 30 {
			start = len(history) - 30
		}
		for i, h := range history[start:] {
			et := L.NewTable()
			et.RawSetString("speaker", lua.LString(h.SpeakerName))
			et.RawSetString("text", lua.LString(h.AllText))
			et.RawSetString("voice", lua.LString(h.VoicePath))
			tbl.RawSetInt(i+1, et)
		}
		L.Push(tbl)
		return 1
	}))

	// ===== 游戏控制 =====
	L.SetGlobal("game_set_auto_play", L.NewFunction(func(L *lua.LState) int {
		lb.eng.autoPlay = L.CheckBool(1)
		return 0
	}))

	L.SetGlobal("game_set_skip_mode", L.NewFunction(func(L *lua.LState) int {
		lb.eng.skipMode = L.CheckBool(1)
		return 0
	}))

	L.SetGlobal("game_get_auto_play", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LBool(lb.eng.autoPlay))
		return 1
	}))

	L.SetGlobal("game_get_skip_mode", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LBool(lb.eng.skipMode))
		return 1
	}))

	L.SetGlobal("game_set_dialog_visible", L.NewFunction(func(L *lua.LState) int {
		lb.eng.dialogVisible = L.CheckBool(1)
		return 0
	}))

	L.SetGlobal("game_toggle_fullscreen", L.NewFunction(func(L *lua.LState) int {
		lb.eng.toggleFullscreen()
		return 0
	}))

	L.SetGlobal("text_is_finished", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LBool(lb.eng.textSys.IsFinished()))
		return 1
	}))

	L.SetGlobal("screen_width", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(lb.eng.width))
		return 1
	}))

	L.SetGlobal("screen_height", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(lb.eng.height))
		return 1
	}))

	// ===== 工具 =====
	L.SetGlobal("log_print", L.NewFunction(func(L *lua.LState) int {
		log.Printf("[Lua] %s", L.CheckString(1))
		return 0
	}))

	// ===== 状态常量 =====
	L.SetGlobal("STATE_TITLE", lua.LNumber(StateTitle))
	L.SetGlobal("STATE_GAME", lua.LNumber(StateGame))
	L.SetGlobal("STATE_SETTINGS", lua.LNumber(StateSettings))
	L.SetGlobal("STATE_SAVE_LOAD", lua.LNumber(StateSaveLoad))
	L.SetGlobal("STATE_GALLERY", lua.LNumber(StateGallery))
	L.SetGlobal("STATE_BACKLOG", lua.LNumber(StateBacklog))
	L.SetGlobal("STATE_MENU", lua.LNumber(StateMenu))

	// 鼠标左键是否持续按住（用于拖动）
	L.SetGlobal("input_left_held", L.NewFunction(func(L *lua.LState) int {
		held := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
		L.Push(lua.LBool(held))
		return 1
	}))

	// 获取鼠标滚轮增量
	L.SetGlobal("input_scroll_delta", L.NewFunction(func(L *lua.LState) int {
		_, dy := ebiten.Wheel()
		L.Push(lua.LNumber(dy))
		return 1
	}))

	// 裁剪区域支持（offscreen渲染）
	var clipOffscreen *ebiten.Image
	var clipOffscreenW, clipOffscreenH int

	L.SetGlobal("clip_begin", L.NewFunction(func(L *lua.LState) int {
		w := int(L.CheckNumber(1))
		h := int(L.CheckNumber(2))
		// 复用或新建offscreen
		if clipOffscreen == nil || clipOffscreenW != w || clipOffscreenH != h {
			clipOffscreen = ebiten.NewImage(w, h)
			clipOffscreenW = w
			clipOffscreenH = h
		}
		clipOffscreen.Clear()
		ud := L.NewUserData()
		ud.Value = clipOffscreen
		L.Push(ud)
		return 1
	}))

	L.SetGlobal("clip_end", L.NewFunction(func(L *lua.LState) int {
		screen := lb.getScreen(L, 1)
		ud, ok := L.Get(2).(*lua.LUserData)
		if !ok || screen == nil {
			return 0
		}
		clipImg, ok := ud.Value.(*ebiten.Image)
		if !ok {
			return 0
		}
		dx := float64(L.CheckNumber(3))
		dy := float64(L.CheckNumber(4))
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(dx, dy)
		screen.DrawImage(clipImg, op)
		return 0
	}))

}

func (lb *LuaBridge) getScreen(L *lua.LState, idx int) *ebiten.Image {
	ud, ok := L.Get(idx).(*lua.LUserData)
	if !ok {
		return nil
	}
	screen, ok := ud.Value.(*ebiten.Image)
	if !ok {
		return nil
	}
	return screen
}

func (lb *LuaBridge) getConfigValue(key string) lua.LValue {
	c := lb.eng.config
	switch key {
	case "text_speed":
		return lua.LNumber(c.TextSpeed)
	case "auto_play_speed":
		return lua.LNumber(c.AutoPlaySpeed)
	case "font_size":
		return lua.LNumber(c.FontSize)
	case "skip_mode":
		return lua.LString(c.SkipMode)
	case "skip_speed":
		return lua.LNumber(c.SkipSpeed)
	case "choice_unset_auto":
		return lua.LBool(c.ChoiceUnsetAuto)
	case "choice_unset_skip":
		return lua.LBool(c.ChoiceUnsetSkip)
	case "mouse_cursor_hide":
		return lua.LNumber(c.MouseCursorHide)
	case "fullscreen":
		return lua.LBool(c.Fullscreen)
	case "master_volume":
		return lua.LNumber(c.MasterVolume)
	case "bgm_volume":
		return lua.LNumber(c.BGMVolume)
	case "se_volume":
		return lua.LNumber(c.SEVolume)
	case "voice_volume":
		return lua.LNumber(c.VoiceVolume)
	case "voice_stop_on_click":
		return lua.LBool(c.VoiceStopOnClick)
	}
	return lua.LNil
}

func (lb *LuaBridge) setConfigValue(key string, val lua.LValue) {
	c := lb.eng.config
	switch key {
	case "text_speed":
		if v, ok := val.(lua.LNumber); ok {
			c.TextSpeed = float64(v)
		}
	case "auto_play_speed":
		if v, ok := val.(lua.LNumber); ok {
			c.AutoPlaySpeed = float64(v)
		}
	case "font_size":
		if v, ok := val.(lua.LNumber); ok {
			c.FontSize = float64(v)
		}
	case "skip_mode":
		if v, ok := val.(lua.LString); ok {
			c.SkipMode = string(v)
		}
	case "skip_speed":
		if v, ok := val.(lua.LNumber); ok {
			c.SkipSpeed = float64(v)
		}
	case "choice_unset_auto":
		if v, ok := val.(lua.LBool); ok {
			c.ChoiceUnsetAuto = bool(v)
		}
	case "choice_unset_skip":
		if v, ok := val.(lua.LBool); ok {
			c.ChoiceUnsetSkip = bool(v)
		}
	case "mouse_cursor_hide":
		if v, ok := val.(lua.LNumber); ok {
			c.MouseCursorHide = float64(v)
			lb.eng.mouseCursorHide = c.MouseCursorHide
		}
	case "fullscreen":
		if v, ok := val.(lua.LBool); ok {
			c.Fullscreen = bool(v)
			ebiten.SetFullscreen(c.Fullscreen)
		}
	case "master_volume":
		if v, ok := val.(lua.LNumber); ok {
			lb.eng.audio.SetMasterVolume(float64(v))
		}
	case "bgm_volume":
		if v, ok := val.(lua.LNumber); ok {
			lb.eng.audio.SetBGMVolume(float64(v))
		}
	case "se_volume":
		if v, ok := val.(lua.LNumber); ok {
			lb.eng.audio.SetSEVolume(float64(v))
		}
	case "voice_volume":
		if v, ok := val.(lua.LNumber); ok {
			lb.eng.audio.SetVoiceVolume(float64(v))
		}
	case "voice_stop_on_click":
		if v, ok := val.(lua.LBool); ok {
			c.VoiceStopOnClick = bool(v)
		}
	}
}

func parseKeyName(name string) ebiten.Key {
	switch strings.ToLower(name) {
	case "enter", "return":
		return ebiten.KeyEnter
	case "space":
		return ebiten.KeySpace
	case "escape", "esc":
		return ebiten.KeyEscape
	case "f1":
		return ebiten.KeyF1
	case "f2":
		return ebiten.KeyF2
	case "f3":
		return ebiten.KeyF3
	case "f4":
		return ebiten.KeyF4
	case "f5":
		return ebiten.KeyF5
	case "f6":
		return ebiten.KeyF6
	case "f7":
		return ebiten.KeyF7
	case "f8":
		return ebiten.KeyF8
	case "f9":
		return ebiten.KeyF9
	case "f10":
		return ebiten.KeyF10
	case "f11":
		return ebiten.KeyF11
	case "up":
		return ebiten.KeyArrowUp
	case "down":
		return ebiten.KeyArrowDown
	case "left":
		return ebiten.KeyArrowLeft
	case "right":
		return ebiten.KeyArrowRight
	}
	return ebiten.KeyEnter
}
