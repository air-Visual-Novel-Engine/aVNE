package engine

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// AnimationType 动画类型
type AnimationType int

const (
	AnimNone AnimationType = iota
	AnimMove
	AnimScale
	AnimFade
	AnimMask
)

// EasingType 缓动类型
type EasingType int

const (
	EaseLinear EasingType = iota
	EaseInOut
	EaseIn
	EaseOut
)

// Animation 动画结构
type Animation struct {
	Type     AnimationType
	Easing   EasingType
	Duration float64
	Elapsed  float64
	Done     bool

	// 移动
	StartX, StartY, EndX, EndY float64

	// 缩放
	StartScaleX, StartScaleY float64
	EndScaleX, EndScaleY     float64

	// 透明度
	StartAlpha, EndAlpha float64

	// 遮罩
	MaskImage *ebiten.Image
	MaskAlpha float64
}

// Layer 层结构
type Layer struct {
	Index   int
	Image   *ebiten.Image
	MaskImg *ebiten.Image
	X, Y    float64
	ScaleX  float64
	ScaleY  float64
	Alpha   float64
	Visible bool
	ZIndex  int

	Animations []*Animation

	// 原始尺寸
	OrigW, OrigH int
}

// NewLayer 创建层
func NewLayer(index int) *Layer {
	return &Layer{
		Index:   index,
		ScaleX:  1.0,
		ScaleY:  1.0,
		Alpha:   1.0,
		Visible: false,
		ZIndex:  index,
	}
}

// LayerManager 层管理器
type LayerManager struct {
	layers  []*Layer
	cache   *Cache
	screenW int
	screenH int
}

// NewLayerManager 创建层管理器
func NewLayerManager(count, w, h int, cache *Cache) *LayerManager {
	lm := &LayerManager{
		layers:  make([]*Layer, count),
		cache:   cache,
		screenW: w,
		screenH: h,
	}
	for i := 0; i < count; i++ {
		lm.layers[i] = NewLayer(i)
	}
	return lm
}

// SetLayerImage 设置层图片
func (lm *LayerManager) SetLayerImage(index int, path string, x, y float64) error {
	if index < 0 || index >= len(lm.layers) {
		return nil
	}
	layer := lm.layers[index]

	if path == "" {
		layer.Image = nil
		layer.Visible = false
		return nil
	}

	img, err := lm.cache.GetImage(path)
	if err != nil {
		return err
	}

	layer.Image = img
	layer.X = x
	layer.Y = y
	layer.Visible = true
	layer.Alpha = 1.0
	layer.ScaleX = 1.0
	layer.ScaleY = 1.0
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	layer.OrigW = w
	layer.OrigH = h

	return nil
}

// SetLayerMask 设置层遮罩（用于过场特效）
func (lm *LayerManager) SetLayerMask(index int, maskPath string) error {
	if index < 0 || index >= len(lm.layers) {
		return nil
	}
	layer := lm.layers[index]

	if maskPath == "" {
		layer.MaskImg = nil
		return nil
	}

	img, err := lm.cache.GetImage(maskPath)
	if err != nil {
		return err
	}
	layer.MaskImg = img
	return nil
}

// AddAnimation 添加动画（支持多个同时进行）
func (lm *LayerManager) AddAnimation(index int, anim *Animation) {
	if index < 0 || index >= len(lm.layers) {
		return
	}
	lm.layers[index].Animations = append(lm.layers[index].Animations, anim)
}

// MoveLayer 移动层
func (lm *LayerManager) MoveLayer(index int, toX, toY, duration float64, easing EasingType) {
	if index < 0 || index >= len(lm.layers) {
		return
	}
	layer := lm.layers[index]
	anim := &Animation{
		Type:     AnimMove,
		Easing:   easing,
		Duration: duration,
		StartX:   layer.X,
		StartY:   layer.Y,
		EndX:     toX,
		EndY:     toY,
	}
	layer.Animations = append(layer.Animations, anim)
}

// ScaleLayer 缩放层
func (lm *LayerManager) ScaleLayer(index int, toScaleX, toScaleY, duration float64, easing EasingType) {
	if index < 0 || index >= len(lm.layers) {
		return
	}
	layer := lm.layers[index]
	anim := &Animation{
		Type:        AnimScale,
		Easing:      easing,
		Duration:    duration,
		StartScaleX: layer.ScaleX,
		StartScaleY: layer.ScaleY,
		EndScaleX:   toScaleX,
		EndScaleY:   toScaleY,
	}
	layer.Animations = append(layer.Animations, anim)
}

// FadeLayer 淡入淡出层
func (lm *LayerManager) FadeLayer(index int, toAlpha, duration float64, easing EasingType) {
	if index < 0 || index >= len(lm.layers) {
		return
	}
	layer := lm.layers[index]
	anim := &Animation{
		Type:       AnimFade,
		Easing:     easing,
		Duration:   duration,
		StartAlpha: layer.Alpha,
		EndAlpha:   toAlpha,
	}
	layer.Animations = append(layer.Animations, anim)
}

// ClearLayer 清除层
func (lm *LayerManager) ClearLayer(index int) {
	if index < 0 || index >= len(lm.layers) {
		return
	}
	lm.layers[index].Image = nil
	lm.layers[index].MaskImg = nil
	lm.layers[index].Visible = false
	lm.layers[index].Animations = nil
}

// IsAnimating 检查是否有动画在进行
func (lm *LayerManager) IsAnimating() bool {
	for _, layer := range lm.layers {
		if len(layer.Animations) > 0 {
			return true
		}
	}
	return false
}

// Update 更新动画
func (lm *LayerManager) Update(delta float64) {
	for _, layer := range lm.layers {
		remaining := layer.Animations[:0]
		for _, anim := range layer.Animations {
			anim.Elapsed += delta
			t := 1.0
			if anim.Duration > 0 {
				t = anim.Elapsed / anim.Duration
			}
			if t > 1.0 {
				t = 1.0
			}
			easedT := applyEasing(t, anim.Easing)

			switch anim.Type {
			case AnimMove:
				layer.X = lerp(anim.StartX, anim.EndX, easedT)
				layer.Y = lerp(anim.StartY, anim.EndY, easedT)
			case AnimScale:
				layer.ScaleX = lerp(anim.StartScaleX, anim.EndScaleX, easedT)
				layer.ScaleY = lerp(anim.StartScaleY, anim.EndScaleY, easedT)
			case AnimFade:
				layer.Alpha = lerp(anim.StartAlpha, anim.EndAlpha, easedT)
			}

			if t < 1.0 {
				remaining = append(remaining, anim)
			}
		}
		layer.Animations = remaining
	}
}

// Draw 渲染所有层
func (lm *LayerManager) Draw(screen *ebiten.Image) {
	for _, layer := range lm.layers {
		if !layer.Visible || layer.Image == nil {
			continue
		}
		lm.drawLayer(screen, layer)
	}
}

func (lm *LayerManager) drawLayer(screen *ebiten.Image, layer *Layer) {
	op := &ebiten.DrawImageOptions{}

	w := float64(layer.OrigW)
	h := float64(layer.OrigH)

	// 以图片中心为原点缩放
	op.GeoM.Translate(-w/2, -h/2)
	op.GeoM.Scale(layer.ScaleX, layer.ScaleY)
	op.GeoM.Translate(w/2, h/2)
	op.GeoM.Translate(layer.X, layer.Y)

	op.ColorScale.ScaleAlpha(float32(layer.Alpha))

	if layer.MaskImg != nil {
		// 使用遮罩绘制（简化实现：使用遮罩作为alpha过渡）
		maskOp := &ebiten.DrawImageOptions{}
		maskOp.GeoM = op.GeoM
		maskOp.ColorM.Scale(0, 0, 0, 1)
		// 实际遮罩效果通过中间纹理实现
		tmpImg := ebiten.NewImage(layer.OrigW, layer.OrigH)
		tmpImg.DrawImage(layer.Image, nil)
		screen.DrawImage(tmpImg, op)
	} else {
		screen.DrawImage(layer.Image, op)
	}
}

// GetLayer 获取层
func (lm *LayerManager) GetLayer(index int) *Layer {
	if index < 0 || index >= len(lm.layers) {
		return nil
	}
	return lm.layers[index]
}

// DrawImageOnLayer 在层上绘制图片（用于过渡）
func (lm *LayerManager) DrawImageOnLayer(index int, img *ebiten.Image, x, y float64, alpha float64) {
	if index < 0 || index >= len(lm.layers) {
		return
	}
	layer := lm.layers[index]
	layer.Image = img
	layer.X = x
	layer.Y = y
	layer.Alpha = alpha
	layer.Visible = true
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	layer.OrigW = w
	layer.OrigH = h
}

// 辅助函数
func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

func applyEasing(t float64, easing EasingType) float64 {
	switch easing {
	case EaseInOut:
		return easeInOut(t)
	case EaseIn:
		return t * t
	case EaseOut:
		return 1 - (1-t)*(1-t)
	default:
		return t
	}
}

func easeInOut(t float64) float64 {
	if t < 0.5 {
		return 2 * t * t
	}
	return 1 - math.Pow(-2*t+2, 2)/2
}

// ColorToFloat 颜色转换辅助
func colorToFloat(c color.Color) (r, g, b, a float64) {
	ri, gi, bi, ai := c.RGBA()
	return float64(ri) / 65535, float64(gi) / 65535, float64(bi) / 65535, float64(ai) / 65535
}
