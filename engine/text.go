package engine

import (
	"bytes"
	"fmt"
	"image/color"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	ebitentext "github.com/hajimehoshi/ebiten/v2/text"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// TextStyle 文字样式
type TextStyle struct {
	FontSize      float64
	Color         color.Color
	Italic        bool
	Strikethrough bool
	Underline     bool
	Superscript   bool
	OutlineColor  color.Color
	OutlineSize   float64
}

// DefaultTextStyle 默认样式
func DefaultTextStyle() TextStyle {
	return TextStyle{
		FontSize:     24,
		Color:        color.White,
		OutlineColor: color.Black,
		OutlineSize:  2,
	}
}

// TextSegment 文字片段
type TextSegment struct {
	Text  string
	Style TextStyle
}

// TextLine 文字行
type TextLine struct {
	Segments    []TextSegment
	SpeakerName string
	AllText     string
	VoicePath   string
}

// TextSystem 文字系统
type TextSystem struct {
	fs     *FileSystem
	cache  *Cache
	config *Config

	// 高质量字体（需要字体文件）
	fontSource *text.GoTextFaceSource

	// 降级字体（内置，无需文件）
	fallbackFace font.Face

	// 是否使用降级字体
	useFallback bool

	currentLine    *TextLine
	displayedChars int
	charTimer      float64
	charInterval   float64
	finished       bool

	fadingOut bool
	fadeAlpha float64
	fadeSpeed float64

	textX, textY float64
	textW, textH float64
	nameX, nameY float64

	history []TextLine
}

// NewTextSystem 创建文字系统
func NewTextSystem(fs *FileSystem, cache *Cache, config *Config) (*TextSystem, error) {
	ts := &TextSystem{
		fs:           fs,
		cache:        cache,
		config:       config,
		charInterval: 1.0 / config.TextSpeed,
		fadeSpeed:    2.0,
		fadeAlpha:    1.0,
		textX:        120,
		textY:        520,
		textW:        1040,
		textH:        160,
		nameX:        120,
		nameY:        490,
		// 默认先用降级字体
		useFallback:  true,
		fallbackFace: basicfont.Face7x13,
	}

	// 尝试加载字体文件（失败则用降级字体）
	if err := ts.loadFont(config.FontPath); err != nil {
		fmt.Printf("[TextSystem] Font load failed (%v), using fallback font\n", err)
		ts.useFallback = true
	}

	return ts, nil
}

// loadFont 加载字体文件
func (ts *TextSystem) loadFont(path string) error {
	data, err := ts.fs.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read font file: %w", err)
	}

	// 验证是否为有效字体
	_, err = opentype.Parse(data)
	if err != nil {
		return fmt.Errorf("parse font: %w", err)
	}

	// 关键修复：使用 bytes.NewReader 而非 strings.NewReader
	src, err := text.NewGoTextFaceSource(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create font source: %w", err)
	}

	ts.fontSource = src
	ts.useFallback = false
	fmt.Printf("[TextSystem] Font loaded: %s\n", path)
	return nil
}

// SetLine 设置当前行
func (ts *TextSystem) SetLine(line *TextLine) {
	if ts.currentLine != nil {
		ts.history = append(ts.history, *ts.currentLine)
		if len(ts.history) > 200 {
			ts.history = ts.history[len(ts.history)-200:]
		}
	}
	ts.currentLine = line
	ts.displayedChars = 0
	ts.charTimer = 0
	ts.finished = false
	ts.fadingOut = false
	ts.fadeAlpha = 1.0
}

// FadeOut 淡出（整行）
func (ts *TextSystem) FadeOut(_ func()) {
	ts.fadingOut = true
	ts.fadeAlpha = 1.0
}

// SkipAll 跳过逐字
func (ts *TextSystem) SkipAll() {
	if ts.currentLine != nil {
		ts.displayedChars = utf8.RuneCountInString(ts.currentLine.AllText)
		ts.finished = true
		ts.charTimer = 0
	}
}

// IsFinished 是否显示完毕
func (ts *TextSystem) IsFinished() bool {
	return ts.finished
}

// InstantShow 立即完整显示当前行（无动画，用于快进）
func (ts *TextSystem) InstantShow() {
	if ts.currentLine == nil {
		return
	}
	// 立即设置为全部字符显示
	total := len([]rune(ts.currentLine.AllText))
	ts.displayedChars = total
	ts.charTimer = 0
	ts.finished = true
	// 取消淡出效果
	ts.fadingOut = false
	ts.fadeAlpha = 1.0
}

// SetInstantMode 设置是否立即显示模式（快进时调用）
func (ts *TextSystem) SetInstantMode(instant bool) {
	if instant && ts.currentLine != nil && !ts.finished {
		ts.InstantShow()
	}
}

// Update 更新文字状态（修正版：快进时直接完成）
func (ts *TextSystem) Update(delta float64) {
	if ts.currentLine == nil {
		return
	}

	if ts.fadingOut {
		ts.fadeAlpha -= delta * ts.fadeSpeed
		if ts.fadeAlpha <= 0 {
			ts.fadeAlpha = 0
			ts.fadingOut = false
			ts.currentLine = nil
		}
		return
	}

	if ts.config.TextSpeed > 0 {
		ts.charInterval = 1.0 / ts.config.TextSpeed
	}

	if !ts.finished {
		ts.charTimer += delta
		for ts.charTimer >= ts.charInterval {
			ts.charTimer -= ts.charInterval
			ts.displayedChars++
			total := len([]rune(ts.currentLine.AllText))
			if ts.displayedChars >= total {
				ts.displayedChars = total
				ts.finished = true
				break
			}
		}
	}
}

// Draw 渲染文字
func (ts *TextSystem) Draw(screen *ebiten.Image) {
	if ts.currentLine == nil {
		return
	}
	alpha := float32(ts.fadeAlpha)
	if alpha <= 0 {
		return
	}

	// 渲染名字
	if ts.currentLine.SpeakerName != "" {
		ts.drawTextWithOutline(screen, ts.currentLine.SpeakerName,
			ts.nameX, ts.nameY, DefaultTextStyle(), alpha)
	}

	// 渲染正文（逐字）
	remaining := ts.displayedChars
	x := ts.textX
	y := ts.textY
	lineHeight := ts.config.FontSize + 8

	for _, seg := range ts.currentLine.Segments {
		if remaining <= 0 {
			break
		}
		runes := []rune(seg.Text)
		visible := len(runes)
		if remaining < visible {
			visible = remaining
		}
		displayText := string(runes[:visible])
		remaining -= visible

		// 换行处理
		lines := ts.wrapText(displayText, seg.Style.FontSize, ts.textW-(x-ts.textX))
		for i, lineStr := range lines {
			if i > 0 {
				x = ts.textX
				y += lineHeight
			}
			ts.drawTextWithOutline(screen, lineStr, x, y, seg.Style, alpha)
			tw, _ := ts.measureText(lineStr, seg.Style.FontSize)
			x += tw
		}
	}
}

// drawTextWithOutline 带描边绘制文字（支持高质量字体和降级字体）
func (ts *TextSystem) drawTextWithOutline(
	screen *ebiten.Image,
	txt string,
	x, y float64,
	style TextStyle,
	alpha float32,
) {
	if txt == "" {
		return
	}

	fontSize := style.FontSize
	if fontSize <= 0 {
		fontSize = ts.config.FontSize
	}

	outlineClr := style.OutlineColor
	if outlineClr == nil {
		outlineClr = color.Black
	}
	mainClr := style.Color
	if mainClr == nil {
		mainClr = color.White
	}

	// =====================
	// 使用高质量字体（GoTextFace）
	// =====================
	if !ts.useFallback && ts.fontSource != nil {
		face := &text.GoTextFace{Source: ts.fontSource, Size: fontSize}

		outlineSize := style.OutlineSize
		if outlineSize <= 0 {
			outlineSize = 2
		}

		offsets := []struct{ dx, dy float64 }{
			{-1, -1}, {0, -1}, {1, -1},
			{-1, 0}, {1, 0},
			{-1, 1}, {0, 1}, {1, 1},
		}

		// 描边
		for _, off := range offsets {
			op := &text.DrawOptions{}
			op.GeoM.Translate(x+off.dx*outlineSize, y+off.dy*outlineSize)
			r, g, b, _ := outlineClr.RGBA()
			op.ColorScale.SetR(float32(r) / 65535)
			op.ColorScale.SetG(float32(g) / 65535)
			op.ColorScale.SetB(float32(b) / 65535)
			op.ColorScale.SetA(alpha * 0.9)
			text.Draw(screen, txt, face, op)
		}

		// 主文字
		op := &text.DrawOptions{}
		op.GeoM.Translate(x, y)
		r, g, b, _ := mainClr.RGBA()
		op.ColorScale.SetR(float32(r) / 65535)
		op.ColorScale.SetG(float32(g) / 65535)
		op.ColorScale.SetB(float32(b) / 65535)
		op.ColorScale.SetA(alpha)
		text.Draw(screen, txt, face, op)

		// 删除线
		if style.Strikethrough {
			tw, th := ts.measureText(txt, fontSize)
			midY := float32(y + th/2)
			cr, cg, cb, _ := mainClr.RGBA()
			vector.StrokeLine(screen,
				float32(x), midY,
				float32(x+tw), midY,
				2,
				color.RGBA{uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8), uint8(float32(255) * alpha)},
				false,
			)
		}
		return
	}

	// =====================
	// 降级字体（basicfont，内置无需文件）
	// =====================
	ts.drawFallbackText(screen, txt, x, y, mainClr, outlineClr, alpha, style)
}

// drawFallbackText 使用内置basicfont绘制（带手动描边）
func (ts *TextSystem) drawFallbackText(
	screen *ebiten.Image,
	txt string,
	x, y float64,
	mainClr, outlineClr color.Color,
	alpha float32,
	style TextStyle,
) {
	face := ts.fallbackFace
	if face == nil {
		face = basicfont.Face7x13
	}

	// 描边（8方向偏移）
	offsets := []struct{ dx, dy int }{
		{-1, -1}, {0, -1}, {1, -1},
		{-1, 0}, {1, 0},
		{-1, 1}, {0, 1}, {1, 1},
	}

	or, og, ob, _ := outlineClr.RGBA()
	oCol := color.RGBA{
		R: uint8(or >> 8),
		G: uint8(og >> 8),
		B: uint8(ob >> 8),
		A: uint8(float32(255) * alpha * 0.9),
	}

	for _, off := range offsets {
		ts.drawBasicText(screen, txt,
			int(x)+off.dx, int(y)+off.dy,
			oCol, face)
	}

	// 主文字
	mr, mg, mb, _ := mainClr.RGBA()
	mCol := color.RGBA{
		R: uint8(mr >> 8),
		G: uint8(mg >> 8),
		B: uint8(mb >> 8),
		A: uint8(float32(255) * alpha),
	}
	ts.drawBasicText(screen, txt, int(x), int(y), mCol, face)

	// 删除线
	if style.Strikethrough {
		metrics := face.Metrics()
		lineY := float32(y) - float32(metrics.Descent.Round())/2
		tw := ts.measureBasicText(txt, face)
		vector.StrokeLine(screen,
			float32(x), lineY,
			float32(x)+float32(tw), lineY,
			1, mCol, false)
	}
}

// drawBasicText 使用basicfont在指定位置绘制文字
func (ts *TextSystem) drawBasicText(
	screen *ebiten.Image,
	txt string,
	x, y int,
	clr color.Color,
	face font.Face,
) {
	// 使用 ebitentext (旧版 text 包) 绘制 basicfont
	op := &ebiten.DrawImageOptions{}
	_ = op
	// 直接使用ebitentext.Draw
	ebitentext.Draw(screen, txt, face, x, y, clr)
}

// measureBasicText 测量basicfont文字宽度
func (ts *TextSystem) measureBasicText(txt string, face font.Face) int {
	d := &font.Drawer{Face: face}
	return d.MeasureString(txt).Round()
}

// measureText 测量文字尺寸
func (ts *TextSystem) measureText(txt string, fontSize float64) (float64, float64) {
	if !ts.useFallback && ts.fontSource != nil {
		face := &text.GoTextFace{Source: ts.fontSource, Size: fontSize}
		w, h := text.Measure(txt, face, fontSize+4)
		return w, h
	}
	// fallback: basicfont每个字符约7px宽，13px高
	w := ts.measureBasicText(txt, basicfont.Face7x13)
	return float64(w), 13
}

// wrapText 文字换行
func (ts *TextSystem) wrapText(txt string, fontSize, maxW float64) []string {
	if maxW <= 0 {
		return []string{txt}
	}
	var lines []string
	runes := []rune(txt)
	start := 0
	for start < len(runes) {
		end := start + 1
		for end <= len(runes) {
			w, _ := ts.measureText(string(runes[start:end]), fontSize)
			if w > maxW {
				break
			}
			end++
		}
		segEnd := end - 1
		if segEnd <= start {
			segEnd = start + 1
		}
		if segEnd > len(runes) {
			segEnd = len(runes)
		}
		lines = append(lines, string(runes[start:segEnd]))
		start = segEnd
	}
	if len(lines) == 0 {
		return []string{txt}
	}
	return lines
}

// GetHistory 获取历史
func (ts *TextSystem) GetHistory() []TextLine {
	return ts.history
}

// ParseLine 解析KrkrStyle标签
func ParseLine(raw string, defaultStyle TextStyle) *TextLine {
	line := &TextLine{}
	style := defaultStyle
	runes := []rune(raw)
	i := 0
	var curText strings.Builder

	flushSeg := func() {
		if curText.Len() > 0 {
			line.Segments = append(line.Segments, TextSegment{
				Text: curText.String(), Style: style,
			})
			line.AllText += curText.String()
			curText.Reset()
		}
	}

	for i < len(runes) {
		if runes[i] == '[' {
			end := i + 1
			for end < len(runes) && runes[end] != ']' {
				end++
			}
			if end < len(runes) {
				tag := string(runes[i+1 : end])
				flushSeg()
				style = applyTag(tag, style, defaultStyle)
				i = end + 1
				continue
			}
		}
		curText.WriteRune(runes[i])
		i++
	}
	flushSeg()
	return line
}

func applyTag(tag string, current TextStyle, def TextStyle) TextStyle {
	tag = strings.TrimSpace(tag)
	lower := strings.ToLower(tag)
	switch {
	case lower == "i":
		current.Italic = true
	case lower == "/i":
		current.Italic = false
	case lower == "s":
		current.Strikethrough = true
	case lower == "/s":
		current.Strikethrough = false
	case lower == "sup":
		current.Superscript = true
	case lower == "/sup":
		current.Superscript = false
	case strings.HasPrefix(lower, "color="):
		current.Color = parseHexColor(strings.TrimPrefix(tag[6:], "#"))
	case lower == "/color":
		current.Color = def.Color
	case strings.HasPrefix(lower, "size="):
		var sz float64
		fmt.Sscanf(tag[5:], "%f", &sz)
		if sz > 0 {
			current.FontSize = sz
		}
	case lower == "/size":
		current.FontSize = def.FontSize
	}
	return current
}

func parseHexColor(hex string) color.Color {
	var r, g, b uint8
	if len(hex) == 6 {
		fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	}
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

// measureStringWidth 辅助：测量字符串像素宽度（用于font.Drawer）
func measureStringWidth(s string, face font.Face) float64 {
	d := font.Drawer{Face: face}
	advance := d.MeasureString(s)
	return float64(advance) / float64(fixed.Int26_6(1<<6))
}
