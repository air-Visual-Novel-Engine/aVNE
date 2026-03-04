package engine

import (
	"fmt"
	"image/color"
	"log"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

// ============================================================
// GameState 游戏状态枚举
// ============================================================
type GameState int

const (
	StateTitle    GameState = iota // 0 标题界面
	StateGame                      // 1 游戏中
	StateSettings                  // 2 设置界面（叠加）
	StateSaveLoad                  // 3 存读档界面
	StateGallery                   // 4 鉴赏界面
	StateBacklog                   // 5 Backlog（叠加）
	StateMenu                      // 6 游戏菜单（叠加）
	StateVideo                     // 7 视频播放
)

// ============================================================
// Transition 场景过渡动画
// ============================================================
type Transition struct {
	active   bool
	alpha    float64 // 0=透明 1=全黑
	speed    float64 // 每秒变化量
	fadeIn   bool    // true=淡入(黑→清晰) false=淡出(清晰→黑)
	callback func()  // 过渡完成后的回调
}

// ============================================================
// Engine 主引擎
// ============================================================
type Engine struct {
	// 核心子系统
	fs        *FileSystem
	cache     *Cache
	config    *Config
	input     *InputManager
	layers    *LayerManager
	textSys   *TextSystem
	script    *ScriptEngine
	audio     *AudioSystem
	video     *VideoPlayer
	saveLoad  *SaveSystem
	gallery   *GallerySystem
	luaBridge *LuaBridge

	// 状态管理
	state     GameState
	prevState GameState

	// 标题界面初始化标记（防止从子界面返回时重置）
	titleInitialized bool
	// 进入子界面时记录来源状态
	sourceStateForSub GameState

	// 画面尺寸
	width  int
	height int

	// 渲染
	mainScreen        *ebiten.Image
	overlayBackground *ebiten.Image // 叠加界面的背景截图
	transition        *Transition
	transitionTo      GameState

	// 对话框
	dialogVisible bool

	// 自动播放
	autoPlay      bool
	autoPlayTimer float64

	// 快进
	skipMode  bool    // F8 手动快进开关
	ctrlSkip  bool    // Ctrl 键快进
	skipTimer float64 // 快进步进计时器

	// 鼠标光标自动隐藏
	mouseCursor      bool
	mouseCursorTimer float64
	mouseCursorHide  float64 // 来自 config，<=0 表示永不隐藏

	// 帧计时
	lastTime    time.Time
	delta       float64
	initialized bool

	// 退出标志
	quitting bool
}

// ============================================================
// 状态分类辅助函数
// ============================================================

// isOverlayState 叠加在截图背景上渲染的界面
func isOverlayState(state GameState) bool {
	return state == StateSettings ||
		state == StateBacklog ||
		state == StateMenu ||
		state == StateSaveLoad // ★ 新增
}

// isSubScreenState 独立渲染的子界面（返回时不重新初始化来源界面）
func isSubScreenState(state GameState) bool {
	return state == StateGallery
}

// ============================================================
// NewEngine 创建并初始化引擎
// ============================================================
func NewEngine() (*Engine, error) {
	eng := &Engine{
		width:            1280,
		height:           720,
		state:            StateTitle,
		prevState:        StateTitle,
		dialogVisible:    true,
		mouseCursor:      true,
		titleInitialized: false,
		lastTime:         time.Now(),
		transition:       &Transition{},
	}

	var err error

	// ── 文件系统（优先读取data.zip，fallback到本地目录）──
	eng.fs, _ = NewFileSystem("data.zip", ".")

	// ── 缓存 ──
	eng.cache = NewCache(eng.fs)

	// ── 配置（从 save/config.json 加载）──
	eng.config, err = NewConfig("save/config.json", eng.fs)
	if err != nil || eng.config == nil {
		log.Printf("[Engine] Config load failed (%v), using defaults", err)
		eng.config = DefaultConfig()
	}

	// ── 同步配置到引擎字段 ──
	eng.applyConfigToEngine()

	// ── 输入管理器 ──
	eng.input = NewInputManager()

	// ── 层管理器 ──
	eng.layers = NewLayerManager(
		eng.config.LayerCount,
		eng.width,
		eng.height,
		eng.cache,
	)

	// ── 文字系统 ──
	eng.textSys, err = NewTextSystem(eng.fs, eng.cache, eng.config)
	if err != nil {
		log.Printf("[Engine] TextSystem init warning: %v", err)
	}

	// ── 音频系统 ──
	eng.audio, err = NewAudioSystem(eng.fs, eng.cache, eng.config)
	if err != nil {
		log.Printf("[Engine] Audio init warning: %v", err)
		eng.audio = &AudioSystem{config: eng.config}
	}

	// ── 视频播放器 ──
	eng.video = NewVideoPlayer(eng.fs)

	// ── 存档系统 ──
	eng.saveLoad, err = NewSaveSystem("save", eng)
	if err != nil {
		return nil, fmt.Errorf("save system: %w", err)
	}

	// ── 鉴赏系统 ──
	eng.gallery, err = NewGallerySystem(eng.fs, "save/gallery.json")
	if err != nil {
		log.Printf("[Engine] Gallery init warning: %v", err)
		eng.gallery = &GallerySystem{
			savePath: "save/gallery.json",
			data: GalleryData{
				CGs:    make(map[string]*CGEntry),
				Events: make(map[string]*EventEntry),
			},
		}
	}

	// ── 渲染缓冲 ──
	eng.mainScreen = ebiten.NewImage(eng.width, eng.height)

	// ── 脚本引擎 ──
	eng.script, err = NewScriptEngine(eng)
	if err != nil {
		return nil, fmt.Errorf("script engine: %w", err)
	}

	// ── Lua 桥接 ──
	eng.luaBridge, err = NewLuaBridge(eng)
	if err != nil {
		return nil, fmt.Errorf("lua bridge: %w", err)
	}

	// ── 加载 Lua 界面文件 ──
	luaFiles := []string{
		"lua/title.lua",
		"lua/settings.lua",
		"lua/save_load.lua",
		"lua/gallery.lua",
		"lua/game_ui.lua",
		"lua/backlog.lua",
	}
	for _, f := range luaFiles {
		if err := eng.luaBridge.LoadFile(f); err != nil {
			log.Printf("[Engine] Lua load warning %s: %v", f, err)
		}
	}

	// ── 应用需要 Ebiten API 的配置 ──
	eng.applyConfigEbiten()

	// ── 初始化标题界面（直接调用，绕过 SetState 避免重复逻辑）──
	eng.luaBridge.CallFunction("title_init")
	eng.titleInitialized = true

	eng.initialized = true
	log.Printf("[Engine] Initialized OK | %dx%d | State=Title", eng.width, eng.height)
	return eng, nil
}

// ============================================================
// Update 每帧更新（ebiten.Game 接口）
// ============================================================
func (eng *Engine) Update() error {
	if eng.quitting {
		return ebiten.Termination
	}

	// 帧时间计算（上限0.1秒防止失焦恢复时跳帧）
	now := time.Now()
	eng.delta = now.Sub(eng.lastTime).Seconds()
	if eng.delta > 0.1 {
		eng.delta = 0.1
	}
	eng.lastTime = now

	// 子系统更新
	eng.input.Update()
	eng.updateMouseCursor()
	if eng.audio != nil {
		eng.audio.Update()
	}

	// 过渡动画期间屏蔽所有输入
	if eng.transition.active {
		eng.updateTransition()
		return nil
	}

	// 全局热键
	if err := eng.handleHotkeys(); err != nil {
		return err
	}

	// 各状态 Update
	switch eng.state {
	case StateTitle:
		eng.luaBridge.CallFunction("title_update", eng.delta)

	case StateGame:
		eng.updateGame()
		eng.handleGameInput()

	case StateSettings:
		eng.luaBridge.CallFunction("settings_update", eng.delta)

	case StateSaveLoad:
		eng.luaBridge.CallFunction("save_load_update", eng.delta)

	case StateGallery:
		eng.luaBridge.CallFunction("gallery_update", eng.delta)

	case StateBacklog:
		eng.luaBridge.CallFunction("backlog_update", eng.delta)

	case StateMenu:
		eng.luaBridge.CallFunction("game_menu_update", eng.delta)

	case StateVideo:
		eng.video.Update(eng.delta)
		if eng.video.Finished() {
			eng.SetState(StateGame)
		}
	}

	eng.layers.Update(eng.delta)
	eng.textSys.Update(eng.delta)

	return nil
}

// ============================================================
// Draw 每帧渲染（ebiten.Game 接口）
// ============================================================
func (eng *Engine) Draw(screen *ebiten.Image) {
	screen.Fill(color.Black)

	switch eng.state {

	// ── 叠加界面：截图背景 + 半透明遮罩 + UI ──
	case StateSettings:
		eng.drawOverlayBackground(screen)
		eng.luaBridge.CallDraw("settings_draw", screen)

	case StateBacklog:
		eng.drawOverlayBackground(screen)
		eng.luaBridge.CallDraw("backlog_draw", screen)

	case StateMenu:
		eng.drawOverlayBackground(screen)
		if eng.dialogVisible {
			eng.luaBridge.CallDraw("game_ui_draw", screen)
			eng.textSys.Draw(screen)
		}
		eng.luaBridge.CallDraw("game_menu_draw", screen)

	// ★ 存读档改为叠加渲染
	case StateSaveLoad:
		eng.drawOverlayBackground(screen)
		eng.luaBridge.CallDraw("save_load_draw", screen)

	// ── 普通界面 ──
	case StateTitle:
		eng.layers.Draw(screen)
		eng.luaBridge.CallDraw("title_draw", screen)

	case StateGame:
		eng.layers.Draw(screen)
		if eng.dialogVisible {
			eng.luaBridge.CallDraw("game_ui_draw", screen)
			eng.textSys.Draw(screen)
		}
		if eng.script.HasChoices() {
			eng.luaBridge.CallDraw("choice_draw", screen)
		}

	case StateGallery:
		eng.layers.Draw(screen)
		eng.luaBridge.CallDraw("gallery_draw", screen)

	case StateVideo:
		eng.video.Draw(screen)
	}

	// ── 过渡遮罩 ──
	if eng.transition.active {
		overlay := ebiten.NewImage(eng.width, eng.height)
		a := uint8(eng.transition.alpha * 255)
		overlay.Fill(color.RGBA{R: 0, G: 0, B: 0, A: a})
		screen.DrawImage(overlay, nil)
	}

	// ── 调试信息 ──
	if eng.config.Debug {
		ebitenutil.DebugPrint(screen, fmt.Sprintf(
			"FPS:%.1f | State:%d Prev:%d | Skip:%v Ctrl:%v Auto:%v | Dialog:%v",
			ebiten.ActualFPS(),
			eng.state, eng.prevState,
			eng.skipMode, eng.ctrlSkip, eng.autoPlay,
			eng.dialogVisible,
		))
	}
}

// Layout 窗口布局（ebiten.Game 接口）
func (eng *Engine) Layout(outsideW, outsideH int) (int, int) {
	return eng.width, eng.height
}

// ============================================================
// SetState 切换游戏状态
// ============================================================
func (eng *Engine) SetState(state GameState) {
	prevIsOverlay := isOverlayState(eng.state)
	nextIsOverlay := isOverlayState(state)
	prevIsSubScreen := isSubScreenState(eng.state)
	nextIsTitle := (state == StateTitle)

	// ── 进入叠加界面前截图 ──
	if nextIsOverlay && !prevIsOverlay {
		eng.captureOverlayBackground()
	}

	// ── 记录进入子界面时的来源状态 ──
	if isSubScreenState(state) {
		eng.sourceStateForSub = eng.state
	}

	eng.prevState = eng.state
	eng.state = state

	// ── 从叠加界面返回：跳过初始化，保持原界面状态 ──
	if prevIsOverlay && !nextIsOverlay {
		log.Printf("[Engine] overlay return: %d -> %d (skip init)", eng.prevState, eng.state)
		return
	}

	// ── 从子界面返回标题：跳过 title_init，保持标题当前状态 ──
	if prevIsSubScreen && nextIsTitle {
		log.Printf("[Engine] sub-screen return to title: %d -> %d (skip init)", eng.prevState, eng.state)
		return
	}

	// ── 正常初始化 ──
	switch state {
	case StateTitle:
		// 首次或强制重置时才初始化
		if !eng.titleInitialized {
			eng.luaBridge.CallFunction("title_init")
			eng.titleInitialized = true
		}

	case StateSettings:
		eng.luaBridge.CallFunction("settings_init")

	case StateSaveLoad:
		eng.luaBridge.CallFunction("save_load_init")

	case StateGallery:
		eng.luaBridge.CallFunction("gallery_init")

	case StateBacklog:
		eng.luaBridge.CallFunction("backlog_init")

	case StateMenu:
		eng.luaBridge.CallFunction("game_menu_init")
	}

	log.Printf("[Engine] State: %d -> %d", eng.prevState, eng.state)
}

// ============================================================
// ForceResetTitle 强制重置标题界面（游戏结束后返回标题时使用）
// ============================================================
func (eng *Engine) ForceResetTitle() {
	eng.titleInitialized = false
	eng.skipMode = false
	eng.ctrlSkip = false
	eng.autoPlay = false
	eng.autoPlayTimer = 0
	eng.skipTimer = 0
	eng.SetState(StateTitle)
}

// ============================================================
// captureOverlayBackground 截图当前帧作为叠加界面背景
// ============================================================
func (eng *Engine) captureOverlayBackground() {
	if eng.overlayBackground == nil {
		eng.overlayBackground = ebiten.NewImage(eng.width, eng.height)
	}
	eng.overlayBackground.Clear()

	switch eng.state {
	case StateTitle:
		eng.overlayBackground.Fill(color.White)
		eng.layers.Draw(eng.overlayBackground)
		eng.luaBridge.CallDraw("title_draw", eng.overlayBackground)

	case StateGame:
		eng.overlayBackground.Fill(color.Black)
		eng.layers.Draw(eng.overlayBackground)
		if eng.dialogVisible {
			eng.luaBridge.CallDraw("game_ui_draw", eng.overlayBackground)
			eng.textSys.Draw(eng.overlayBackground)
		}
		if eng.script.HasChoices() {
			eng.luaBridge.CallDraw("choice_draw", eng.overlayBackground)
		}

	case StateGallery:
		eng.overlayBackground.Fill(color.Black)
		eng.layers.Draw(eng.overlayBackground)
		eng.luaBridge.CallDraw("gallery_draw", eng.overlayBackground)

	// ★ StateSaveLoad 已是叠加界面，不再需要独立截图分支
	// （从 SaveLoad 打开 Settings 时，截图的是 SaveLoad 当前画面）

	default:
		eng.overlayBackground.Fill(color.Black)
		eng.layers.Draw(eng.overlayBackground)
	}
}

// ============================================================
// drawOverlayBackground 绘制叠加背景（截图 + 半透明遮罩）
// ============================================================
func (eng *Engine) drawOverlayBackground(screen *ebiten.Image) {
	if eng.overlayBackground != nil {
		screen.DrawImage(eng.overlayBackground, nil)
	} else {
		screen.Fill(color.Black)
	}
	// 半透明深色遮罩，让UI面板更清晰
	mask := ebiten.NewImage(eng.width, eng.height)
	mask.Fill(color.RGBA{R: 0, G: 0, B: 0, A: 130})
	screen.DrawImage(mask, nil)
}

// ============================================================
// StartTransition 标准过渡（约0.67秒）
// ============================================================
func (eng *Engine) StartTransition(to GameState, fadeOut bool) {
	eng.startTransitionInternal(to, fadeOut, 1.5)
}

// ============================================================
// StartTransitionSlow 慢速过渡（自定义时长，用于开场等）
// ============================================================
func (eng *Engine) StartTransitionSlow(to GameState, duration float64) {
	if duration <= 0 {
		duration = 5.0
	}
	eng.startTransitionInternal(to, true, 1.0/duration)
}

// ============================================================
// StartTransitionWithCallback 带回调的过渡
// ============================================================
func (eng *Engine) StartTransitionWithCallback(to GameState, duration float64, cb func()) {
	if duration <= 0 {
		duration = 5.0
	}
	eng.transition.active = true
	eng.transitionTo = to
	eng.transition.speed = 1.0 / duration
	eng.transition.alpha = 0
	eng.transition.fadeIn = false
	eng.transition.callback = cb
}

func (eng *Engine) startTransitionInternal(to GameState, fadeOut bool, speed float64) {
	eng.transition.active = true
	eng.transitionTo = to
	eng.transition.speed = speed
	if fadeOut {
		eng.transition.alpha = 0
		eng.transition.fadeIn = false
	} else {
		eng.transition.alpha = 1
		eng.transition.fadeIn = true
	}
	eng.transition.callback = nil
}

func (eng *Engine) updateTransition() {
	if eng.transition.fadeIn {
		// 淡入：alpha 从1降到0（黑→清晰）
		eng.transition.alpha -= eng.delta * eng.transition.speed
		if eng.transition.alpha <= 0 {
			eng.transition.alpha = 0
			eng.transition.active = false
			if eng.transition.callback != nil {
				cb := eng.transition.callback
				eng.transition.callback = nil
				cb()
			}
		}
	} else {
		// 淡出：alpha 从0升到1（清晰→黑）
		eng.transition.alpha += eng.delta * eng.transition.speed
		if eng.transition.alpha >= 1 {
			eng.transition.alpha = 1
			eng.SetState(eng.transitionTo)
			eng.transition.fadeIn = true
		}
	}
}

// ============================================================
// updateGame 游戏状态每帧逻辑
// ============================================================
func (eng *Engine) updateGame() {
	isSkipping := eng.skipMode || eng.ctrlSkip

	// ── 自动播放（快进时禁用）──
	if eng.autoPlay && !isSkipping && !eng.script.HasChoices() {
		if eng.textSys.IsFinished() {
			eng.autoPlayTimer += eng.delta
			autoSpeed := eng.config.AutoPlaySpeed
			if autoSpeed <= 0 {
				autoSpeed = 3.0
			}
			if eng.autoPlayTimer >= autoSpeed {
				eng.autoPlayTimer = 0
				eng.script.Advance()
			}
		}
	}

	// ── 快进（F8 或 Ctrl）──
	if isSkipping {
		eng.autoPlayTimer = 0

		skipDelay := eng.config.SkipSpeed
		if skipDelay <= 0 {
			skipDelay = 0.03
		}
		if eng.ctrlSkip {
			skipDelay = 0.01 // Ctrl 快进更激进
		}

		eng.skipTimer += eng.delta
		if eng.skipTimer >= skipDelay {
			eng.skipTimer = 0
			if !eng.textSys.IsFinished() {
				// ★ 快进时文字立即完整显示，无逐字动画
				eng.textSys.InstantShow()
			} else if !eng.script.HasChoices() {
				eng.script.Advance()
			}
		}
	} else {
		eng.skipTimer = 0
	}

	eng.script.Update(eng.delta)
}

// ============================================================
// handleGameInput 游戏中的输入处理
// ============================================================
func (eng *Engine) handleGameInput() {
	if eng.state != StateGame {
		return
	}

	// Ctrl 快进（按住持续，松开停止）
	eng.ctrlSkip = eng.input.IsCtrlHeld()

	// 滚轮向上：打开 Backlog
	if eng.input.IsScrollingUp() {
		eng.SetState(StateBacklog)
		return
	}

	// 滚轮向下：推进文字
	if eng.input.IsScrollingDown() {
		if !eng.script.HasChoices() {
			if !eng.textSys.IsFinished() {
				eng.textSys.InstantShow()
			} else {
				eng.script.Advance()
			}
		}
		return
	}

	// 右键：打开游戏菜单
	if eng.input.IsRightClickJustPressed() {
		eng.SetState(StateMenu)
		return
	}

	// Space/F6：隐藏/显示对话框
	if eng.input.IsHideDialogInput() {
		eng.dialogVisible = !eng.dialogVisible
		return
	}

	// 左键/Enter：推进文字
	if eng.input.IsLeftClickJustPressed() || eng.input.IsEnterJustPressed() {
		if !eng.script.HasChoices() {
			if !eng.textSys.IsFinished() {
				eng.textSys.InstantShow()
			} else {
				eng.script.Advance()
			}
		}
	}
}

// ============================================================
// handleHotkeys 全局热键
// ============================================================
func (eng *Engine) handleHotkeys() error {
	// F1 / Shift：设置
	if eng.input.IsFKeyJustPressed(1) || eng.input.IsShiftJustPressed() {
		if eng.state != StateSettings {
			eng.SetState(StateSettings)
		}
		return nil
	}

	// ★ F2：存档界面（仅游戏中才能存档，标题界面直接进读档）
	if eng.input.IsFKeyJustPressed(2) {
		if eng.state == StateGame {
			eng.luaBridge.CallFunction("save_load_mode", "save")
		} else {
			eng.luaBridge.CallFunction("save_load_mode", "load")
		}
		if eng.state != StateSaveLoad {
			eng.SetState(StateSaveLoad)
		}
		return nil
	}

	// F3：读档界面
	if eng.input.IsFKeyJustPressed(3) {
		eng.luaBridge.CallFunction("save_load_mode", "load")
		if eng.state != StateSaveLoad {
			eng.SetState(StateSaveLoad)
		}
		return nil
	}

	// F4：重播人声
	if eng.input.IsFKeyJustPressed(4) {
		if eng.audio != nil {
			eng.audio.ReplayVoice()
		}
	}

	// F5：自动播放开关
	if eng.input.IsFKeyJustPressed(5) {
		eng.autoPlay = !eng.autoPlay
		eng.autoPlayTimer = 0
		log.Printf("[Engine] AutoPlay: %v", eng.autoPlay)
	}

	// F6：隐藏/显示对话框（仅游戏中）
	if eng.input.IsFKeyJustPressed(6) {
		if eng.state == StateGame {
			eng.dialogVisible = !eng.dialogVisible
		}
	}

	// F7：切换全屏
	if eng.input.IsFKeyJustPressed(7) {
		eng.toggleFullscreen()
	}

	// F8：手动快进开关
	if eng.input.IsFKeyJustPressed(8) {
		eng.skipMode = !eng.skipMode
		eng.skipTimer = 0
		eng.autoPlayTimer = 0
		log.Printf("[Engine] SkipMode: %v", eng.skipMode)
	}

	// F9：快速存档（仅游戏中）
	if eng.input.IsFKeyJustPressed(9) {
		if eng.state == StateGame && eng.saveLoad != nil {
			eng.saveLoad.QuickSave(eng)
		}
	}

	// F10：Backlog（仅游戏中）
	if eng.input.IsFKeyJustPressed(10) {
		if eng.state == StateGame {
			eng.SetState(StateBacklog)
		}
	}

	// F11：快速读档
	if eng.input.IsFKeyJustPressed(11) {
		if eng.saveLoad != nil {
			eng.saveLoad.QuickLoad(eng)
		}
	}

	// Alt+Enter：切换全屏
	if eng.input.IsAltEnter() {
		eng.toggleFullscreen()
	}

	return nil
}

// ============================================================
// updateMouseCursor 鼠标光标自动隐藏
// ============================================================
func (eng *Engine) updateMouseCursor() {
	// <=0 表示永不隐藏
	if eng.mouseCursorHide <= 0 {
		if !eng.mouseCursor {
			eng.mouseCursor = true
			ebiten.SetCursorMode(ebiten.CursorModeVisible)
		}
		return
	}

	// 使用 InputManager.IsMouseMoved()
	// Update() 已正确计算：last=上帧位置，cur=本帧位置
	if eng.input.IsMouseMoved() {
		// 鼠标移动：重置计时器，恢复显示
		eng.mouseCursorTimer = 0
		if !eng.mouseCursor {
			eng.mouseCursor = true
			ebiten.SetCursorMode(ebiten.CursorModeVisible)
		}
	} else {
		// 鼠标静止：累计计时
		eng.mouseCursorTimer += eng.delta
		if eng.mouseCursorTimer >= eng.mouseCursorHide && eng.mouseCursor {
			eng.mouseCursor = false
			ebiten.SetCursorMode(ebiten.CursorModeHidden)
		}
	}
}

// ============================================================
// applyConfigToEngine 配置值同步到引擎字段
// ============================================================
func (eng *Engine) applyConfigToEngine() {
	eng.mouseCursorHide = eng.config.MouseCursorHide
	if eng.config.WindowW > 0 {
		eng.width = eng.config.WindowW
	}
	if eng.config.WindowH > 0 {
		eng.height = eng.config.WindowH
	}
	log.Printf(
		"[Engine] Config: textSpeed=%.1f autoSpeed=%.1f "+
			"bgmVol=%.2f seVol=%.2f voiceVol=%.2f masterVol=%.2f "+
			"fullscreen=%v mouseHide=%.1f",
		eng.config.TextSpeed,
		eng.config.AutoPlaySpeed,
		eng.config.BGMVolume,
		eng.config.SEVolume,
		eng.config.VoiceVolume,
		eng.config.MasterVolume,
		eng.config.Fullscreen,
		eng.config.MouseCursorHide,
	)
}

// ============================================================
// applyConfigEbiten 应用需要 Ebiten API 的配置
// ============================================================
func (eng *Engine) applyConfigEbiten() {
	if eng.config.Fullscreen {
		ebiten.SetFullscreen(true)
		log.Printf("[Engine] Fullscreen enabled")
	}
	if eng.config.WindowW > 0 && eng.config.WindowH > 0 {
		ebiten.SetWindowSize(eng.config.WindowW, eng.config.WindowH)
	}
}

// ============================================================
// ReloadConfig 运行时重新应用配置（续）
// ============================================================
func (eng *Engine) ReloadConfig() {
	eng.applyConfigToEngine()
	eng.applyConfigEbiten()
	if eng.audio != nil {
		eng.audio.SetMasterVolume(eng.config.MasterVolume)
		eng.audio.SetBGMVolume(eng.config.BGMVolume)
		eng.audio.SetSEVolume(eng.config.SEVolume)
		eng.audio.SetVoiceVolume(eng.config.VoiceVolume)
	}
	log.Printf("[Engine] Config reloaded and applied")
}

// ============================================================
// toggleFullscreen 切换全屏
// ============================================================
func (eng *Engine) toggleFullscreen() {
	eng.config.Fullscreen = !eng.config.Fullscreen
	ebiten.SetFullscreen(eng.config.Fullscreen)
	if err := eng.config.Save(); err != nil {
		log.Printf("[Engine] Config save failed: %v", err)
	}
	log.Printf("[Engine] Fullscreen: %v", eng.config.Fullscreen)
}

// ============================================================
// GetScreenshot 获取当前层的缩略图（用于存档预览）
// ============================================================
func (eng *Engine) GetScreenshot() *ebiten.Image {
	thumb := ebiten.NewImage(320, 180)
	thumb.Fill(color.Black)
	eng.layers.Draw(thumb)
	return thumb
}

// ============================================================
// GetPrevState 获取上一个状态（供 Lua 调用）
// ============================================================
func (eng *Engine) GetPrevState() GameState {
	return eng.prevState
}

// ============================================================
// IsQuitting 是否正在退出
// ============================================================
func (eng *Engine) IsQuitting() bool {
	return eng.quitting
}
