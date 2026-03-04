package engine

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// InputManager 输入管理器
type InputManager struct {
	// 当前帧鼠标位置
	curMouseX int
	curMouseY int
	// 上一帧鼠标位置（Update时先保存再读取，确保移动检测正确）
	lastMouseX int
	lastMouseY int
	// 本帧鼠标是否移动
	mouseMoved bool

	// 本帧滚轮增量
	scrollDeltaX float64
	scrollDeltaY float64
}

// NewInputManager 创建输入管理器
func NewInputManager() *InputManager {
	x, y := ebiten.CursorPosition()
	return &InputManager{
		curMouseX:  x,
		curMouseY:  y,
		lastMouseX: x,
		lastMouseY: y,
	}
}

// Update 每帧更新（必须在帧最开始、其他所有输入判断之前调用）
// 关键：先保存上一帧位置，再读取当前帧位置，保证移动检测正确
func (im *InputManager) Update() {
	// ★ 先保存上帧位置
	im.lastMouseX = im.curMouseX
	im.lastMouseY = im.curMouseY

	// ★ 再读取当前帧位置
	im.curMouseX, im.curMouseY = ebiten.CursorPosition()

	// 计算本帧是否移动
	im.mouseMoved = (im.curMouseX != im.lastMouseX ||
		im.curMouseY != im.lastMouseY)

	// 读取滚轮增量
	dx, dy := ebiten.Wheel()
	im.scrollDeltaX = dx
	im.scrollDeltaY = dy
}

// ============================================================
// 鼠标位置
// ============================================================

// MousePosition 获取当前帧鼠标位置（int）
func (im *InputManager) MousePosition() (int, int) {
	return im.curMouseX, im.curMouseY
}

// MousePositionF 获取当前帧鼠标位置（float64）
func (im *InputManager) MousePositionF() (float64, float64) {
	return float64(im.curMouseX), float64(im.curMouseY)
}

// LastMousePosition 获取上一帧鼠标位置
func (im *InputManager) LastMousePosition() (int, int) {
	return im.lastMouseX, im.lastMouseY
}

// IsMouseMoved 本帧鼠标是否发生移动
// 使用 curPos != lastPos（Update内已正确计算）
func (im *InputManager) IsMouseMoved() bool {
	return im.mouseMoved
}

// ============================================================
// 鼠标区域检测
// ============================================================

// IsMouseOver 鼠标是否在矩形内（int参数）
func (im *InputManager) IsMouseOver(x, y, w, h int) bool {
	return im.curMouseX >= x && im.curMouseX <= x+w &&
		im.curMouseY >= y && im.curMouseY <= y+h
}

// IsMouseOverF 鼠标是否在矩形内（float64参数，供Lua调用）
func (im *InputManager) IsMouseOverF(x, y, w, h float64) bool {
	fx := float64(im.curMouseX)
	fy := float64(im.curMouseY)
	return fx >= x && fx <= x+w && fy >= y && fy <= y+h
}

// ============================================================
// 鼠标左键
// ============================================================

// IsLeftClickJustPressed 左键本帧刚按下
func (im *InputManager) IsLeftClickJustPressed() bool {
	return inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
}

// IsLeftClickJustReleased 左键本帧刚松开
func (im *InputManager) IsLeftClickJustReleased() bool {
	return inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)
}

// IsLeftClickHeld 左键持续按住
func (im *InputManager) IsLeftClickHeld() bool {
	return ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
}

// ============================================================
// 鼠标右键
// ============================================================

// IsRightClickJustPressed 右键本帧刚按下
func (im *InputManager) IsRightClickJustPressed() bool {
	return inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight)
}

// IsRightClickJustReleased 右键本帧刚松开
func (im *InputManager) IsRightClickJustReleased() bool {
	return inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonRight)
}

// IsRightClickHeld 右键持续按住
func (im *InputManager) IsRightClickHeld() bool {
	return ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight)
}

// ============================================================
// 鼠标中键
// ============================================================

// IsMiddleClickJustPressed 中键本帧刚按下
func (im *InputManager) IsMiddleClickJustPressed() bool {
	return inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonMiddle)
}

// IsMiddleClickHeld 中键持续按住
func (im *InputManager) IsMiddleClickHeld() bool {
	return ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle)
}

// ============================================================
// 鼠标滚轮
// ============================================================

// ScrollDeltaX 本帧水平滚轮增量（向右为正）
func (im *InputManager) ScrollDeltaX() float64 {
	return im.scrollDeltaX
}

// ScrollDeltaY 本帧垂直滚轮增量（向上为正，向下为负）
func (im *InputManager) ScrollDeltaY() float64 {
	return im.scrollDeltaY
}

// IsScrollingUp 本帧是否向上滚动
func (im *InputManager) IsScrollingUp() bool {
	return im.scrollDeltaY > 0
}

// IsScrollingDown 本帧是否向下滚动
func (im *InputManager) IsScrollingDown() bool {
	return im.scrollDeltaY < 0
}

// ============================================================
// 键盘通用
// ============================================================

// IsKeyJustPressed 指定按键本帧刚按下
func (im *InputManager) IsKeyJustPressed(key ebiten.Key) bool {
	return inpututil.IsKeyJustPressed(key)
}

// IsKeyJustReleased 指定按键本帧刚松开
func (im *InputManager) IsKeyJustReleased(key ebiten.Key) bool {
	return inpututil.IsKeyJustReleased(key)
}

// IsKeyHeld 指定按键持续按住
func (im *InputManager) IsKeyHeld(key ebiten.Key) bool {
	return ebiten.IsKeyPressed(key)
}

// IsAnyKeyJustPressed 本帧是否有任意键被按下
func (im *InputManager) IsAnyKeyJustPressed() bool {
	return len(inpututil.AppendJustPressedKeys(nil)) > 0
}

// ============================================================
// 修饰键
// ============================================================

// IsCtrlHeld Ctrl键持续按住（左Ctrl 或 右Ctrl）
func (im *InputManager) IsCtrlHeld() bool {
	return ebiten.IsKeyPressed(ebiten.KeyControlLeft) ||
		ebiten.IsKeyPressed(ebiten.KeyControlRight)
}

// IsCtrlJustPressed Ctrl键本帧刚按下
func (im *InputManager) IsCtrlJustPressed() bool {
	return inpututil.IsKeyJustPressed(ebiten.KeyControlLeft) ||
		inpututil.IsKeyJustPressed(ebiten.KeyControlRight)
}

// IsShiftHeld Shift键持续按住（左Shift 或 右Shift）
func (im *InputManager) IsShiftHeld() bool {
	return ebiten.IsKeyPressed(ebiten.KeyShiftLeft) ||
		ebiten.IsKeyPressed(ebiten.KeyShiftRight)
}

// IsShiftJustPressed Shift键本帧刚按下
func (im *InputManager) IsShiftJustPressed() bool {
	return inpututil.IsKeyJustPressed(ebiten.KeyShiftLeft) ||
		inpututil.IsKeyJustPressed(ebiten.KeyShiftRight)
}

// IsAltHeld Alt键持续按住（左Alt 或 右Alt）
func (im *InputManager) IsAltHeld() bool {
	return ebiten.IsKeyPressed(ebiten.KeyAltLeft) ||
		ebiten.IsKeyPressed(ebiten.KeyAltRight)
}

// IsAltJustPressed Alt键本帧刚按下
func (im *InputManager) IsAltJustPressed() bool {
	return inpututil.IsKeyJustPressed(ebiten.KeyAltLeft) ||
		inpututil.IsKeyJustPressed(ebiten.KeyAltRight)
}

// ============================================================
// 常用功能键
// ============================================================

// IsEnterJustPressed Enter键本帧刚按下（主键盘Enter 或 小键盘Enter）
func (im *InputManager) IsEnterJustPressed() bool {
	return inpututil.IsKeyJustPressed(ebiten.KeyEnter) ||
		inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter)
}

// IsEscapeJustPressed ESC键本帧刚按下
func (im *InputManager) IsEscapeJustPressed() bool {
	return inpututil.IsKeyJustPressed(ebiten.KeyEscape)
}

// IsSpaceJustPressed 空格键本帧刚按下
func (im *InputManager) IsSpaceJustPressed() bool {
	return inpututil.IsKeyJustPressed(ebiten.KeySpace)
}

// IsSpaceHeld 空格键持续按住
func (im *InputManager) IsSpaceHeld() bool {
	return ebiten.IsKeyPressed(ebiten.KeySpace)
}

// IsBackspaceJustPressed Backspace键本帧刚按下
func (im *InputManager) IsBackspaceJustPressed() bool {
	return inpututil.IsKeyJustPressed(ebiten.KeyBackspace)
}

// IsTabJustPressed Tab键本帧刚按下
func (im *InputManager) IsTabJustPressed() bool {
	return inpututil.IsKeyJustPressed(ebiten.KeyTab)
}

// ============================================================
// 方向键
// ============================================================

// IsUpJustPressed 上方向键本帧刚按下
func (im *InputManager) IsUpJustPressed() bool {
	return inpututil.IsKeyJustPressed(ebiten.KeyArrowUp)
}

// IsDownJustPressed 下方向键本帧刚按下
func (im *InputManager) IsDownJustPressed() bool {
	return inpututil.IsKeyJustPressed(ebiten.KeyArrowDown)
}

// IsLeftJustPressed 左方向键本帧刚按下
func (im *InputManager) IsLeftJustPressed() bool {
	return inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft)
}

// IsRightJustPressed 右方向键本帧刚按下
func (im *InputManager) IsRightJustPressed() bool {
	return inpututil.IsKeyJustPressed(ebiten.KeyArrowRight)
}

// IsUpHeld 上方向键持续按住
func (im *InputManager) IsUpHeld() bool {
	return ebiten.IsKeyPressed(ebiten.KeyArrowUp)
}

// IsDownHeld 下方向键持续按住
func (im *InputManager) IsDownHeld() bool {
	return ebiten.IsKeyPressed(ebiten.KeyArrowDown)
}

// IsLeftHeld 左方向键持续按住
func (im *InputManager) IsLeftHeld() bool {
	return ebiten.IsKeyPressed(ebiten.KeyArrowLeft)
}

// IsRightHeld 右方向键持续按住
func (im *InputManager) IsRightHeld() bool {
	return ebiten.IsKeyPressed(ebiten.KeyArrowRight)
}

// ============================================================
// F键组（F1 ~ F12）
// ============================================================

// IsFKeyJustPressed F1~F12 本帧刚按下（n: 1~12）
func (im *InputManager) IsFKeyJustPressed(n int) bool {
	fkeys := []ebiten.Key{
		ebiten.KeyF1, ebiten.KeyF2, ebiten.KeyF3, ebiten.KeyF4,
		ebiten.KeyF5, ebiten.KeyF6, ebiten.KeyF7, ebiten.KeyF8,
		ebiten.KeyF9, ebiten.KeyF10, ebiten.KeyF11, ebiten.KeyF12,
	}
	if n < 1 || n > 12 {
		return false
	}
	return inpututil.IsKeyJustPressed(fkeys[n-1])
}

// IsFKeyHeld F1~F12 持续按住（n: 1~12）
func (im *InputManager) IsFKeyHeld(n int) bool {
	fkeys := []ebiten.Key{
		ebiten.KeyF1, ebiten.KeyF2, ebiten.KeyF3, ebiten.KeyF4,
		ebiten.KeyF5, ebiten.KeyF6, ebiten.KeyF7, ebiten.KeyF8,
		ebiten.KeyF9, ebiten.KeyF10, ebiten.KeyF11, ebiten.KeyF12,
	}
	if n < 1 || n > 12 {
		return false
	}
	return ebiten.IsKeyPressed(fkeys[n-1])
}

// ============================================================
// 组合键
// ============================================================

// IsAltEnter Alt+Enter（常用于切换全屏）
func (im *InputManager) IsAltEnter() bool {
	return im.IsAltHeld() && im.IsEnterJustPressed()
}

// IsCtrlEnter Ctrl+Enter
func (im *InputManager) IsCtrlEnter() bool {
	return im.IsCtrlHeld() && im.IsEnterJustPressed()
}

// IsCtrlS Ctrl+S
func (im *InputManager) IsCtrlS() bool {
	return im.IsCtrlHeld() && inpututil.IsKeyJustPressed(ebiten.KeyS)
}

// IsCtrlZ Ctrl+Z
func (im *InputManager) IsCtrlZ() bool {
	return im.IsCtrlHeld() && inpututil.IsKeyJustPressed(ebiten.KeyZ)
}

// IsCtrlA Ctrl+A
func (im *InputManager) IsCtrlA() bool {
	return im.IsCtrlHeld() && inpututil.IsKeyJustPressed(ebiten.KeyA)
}

// ============================================================
// 游戏专用快捷方法
// ============================================================

// IsAdvanceInput 推进文字的输入（左键 / Enter / 滚轮向下）
func (im *InputManager) IsAdvanceInput() bool {
	return im.IsLeftClickJustPressed() ||
		im.IsEnterJustPressed() ||
		im.IsScrollingDown()
}

// IsSkipInput 快进输入（Ctrl持续按住）
func (im *InputManager) IsSkipInput() bool {
	return im.IsCtrlHeld()
}

// IsOpenBacklogInput 打开Backlog（滚轮向上）
func (im *InputManager) IsOpenBacklogInput() bool {
	return im.IsScrollingUp()
}

// IsOpenMenuInput 打开菜单（右键）
func (im *InputManager) IsOpenMenuInput() bool {
	return im.IsRightClickJustPressed()
}

// IsHideDialogInput 隐藏/显示对话框（Space 或 F6）
func (im *InputManager) IsHideDialogInput() bool {
	return im.IsSpaceJustPressed() ||
		inpututil.IsKeyJustPressed(ebiten.KeyF6)
}

// IsBackInput 返回操作（ESC 或 右键）
func (im *InputManager) IsBackInput() bool {
	return im.IsEscapeJustPressed() || im.IsRightClickJustPressed()
}

// IsConfirmInput 确认操作（Enter 或 左键）
func (im *InputManager) IsConfirmInput() bool {
	return im.IsEnterJustPressed() || im.IsLeftClickJustPressed()
}

// ============================================================
// 调试信息
// ============================================================

// DebugString 返回当前输入状态字符串（用于调试显示）
func (im *InputManager) DebugString() string {
	pressed := inpututil.AppendJustPressedKeys(nil)
	return fmt.Sprintf(
		"Mouse cur(%d,%d) last(%d,%d) moved=%v | Scroll(%.1f,%.1f) | Keys:%v | Ctrl:%v Shift:%v Alt:%v",
		im.curMouseX, im.curMouseY,
		im.lastMouseX, im.lastMouseY,
		im.mouseMoved,
		im.scrollDeltaX, im.scrollDeltaY,
		pressed,
		im.IsCtrlHeld(),
		im.IsShiftHeld(),
		im.IsAltHeld(),
	)
}
