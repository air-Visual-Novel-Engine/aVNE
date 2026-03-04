package engine

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"sort"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

// VideoFrame 视频帧
type VideoFrame struct {
	Image *ebiten.Image
	PTS   float64 // 显示时间戳（秒）
}

// VideoPlayer 视频播放器（纯Go实现，支持图片序列帧格式）
// 注意：由于不使用ffmpeg，此处实现基于APNG/GIF/图片序列帧
type VideoPlayer struct {
	fs       *FileSystem
	frames   []*VideoFrame
	current  int
	elapsed  float64
	playing  bool
	finished bool
	fps      float64
	mu       sync.Mutex

	// 当前帧图
	currentFrame *ebiten.Image

	// 屏幕尺寸
	screenW, screenH int
}

// NewVideoPlayer 创建视频播放器
func NewVideoPlayer(fs *FileSystem) *VideoPlayer {
	return &VideoPlayer{
		fs:      fs,
		fps:     24.0,
		screenW: 1280,
		screenH: 720,
	}
}

// Load 加载视频文件
// 支持：
// 1. .gif 文件（使用image/gif解码）
// 2. 图片序列帧目录（video/xxx/001.png, 002.png...）
// 3. .apng（解析PNG帧）
func (vp *VideoPlayer) Load(path string) error {
	vp.mu.Lock()
	defer vp.mu.Unlock()

	vp.frames = nil
	vp.current = 0
	vp.elapsed = 0
	vp.finished = false
	vp.playing = false

	ext := getExtension(path)
	switch ext {
	case "gif":
		return vp.loadGIF(path)
	case "mp4", "wmv":
		// 尝试加载同名图片序列帧目录
		// 例如: video/intro.mp4 -> 尝试加载 video/intro/ 目录
		seqDir := path[:len(path)-len(ext)-1]
		if err := vp.loadImageSequence(seqDir); err != nil {
			return fmt.Errorf("video %s: no sequence found at %s: %w", path, seqDir, err)
		}
		return nil
	default:
		// 尝试作为图片序列帧目录
		return vp.loadImageSequence(path)
	}
}

func (vp *VideoPlayer) loadGIF(path string) error {
	data, err := vp.fs.ReadFile(path)
	if err != nil {
		return err
	}

	gifImg, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode gif: %w", err)
	}

	// 构建帧
	bounds := image.Rect(0, 0, gifImg.Config.Width, gifImg.Config.Height)
	canvas := image.NewRGBA(bounds)
	pts := 0.0

	for i, frame := range gifImg.Image {
		// 处置方式
		switch gifImg.Disposal[i] {
		case gif.DisposalBackground:
			for y := frame.Bounds().Min.Y; y < frame.Bounds().Max.Y; y++ {
				for x := frame.Bounds().Min.X; x < frame.Bounds().Max.X; x++ {
					canvas.Set(x, y, color.Transparent)
				}
			}
		case gif.DisposalPrevious:
			// 保留前一帧
		}

		// 绘制当前帧
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)

		// 转为ebiten图片
		eImg := ebiten.NewImageFromImage(canvas)
		delay := float64(gifImg.Delay[i]) * 0.01 // 单位：10ms -> 秒

		vp.frames = append(vp.frames, &VideoFrame{
			Image: eImg,
			PTS:   pts,
		})
		pts += delay
	}

	if len(vp.frames) > 0 {
		vp.fps = float64(len(gifImg.Image)) / pts
	}

	return nil
}

func (vp *VideoPlayer) loadImageSequence(dir string) error {
	files, err := vp.fs.ListDir(dir)
	if err != nil || len(files) == 0 {
		return fmt.Errorf("no frames in directory: %s", dir)
	}

	// 排序文件
	sortedFiles := sortFileNames(files)

	frameInterval := 1.0 / vp.fps
	pts := 0.0

	for _, f := range sortedFiles {
		ext := getExtension(f)
		if ext != "png" && ext != "jpg" && ext != "jpeg" {
			continue
		}

		imgPath := dir + "/" + f
		img, err := vp.fs.ReadFile(imgPath)
		if err != nil {
			continue
		}

		decoded, _, err := image.Decode(bytes.NewReader(img))
		if err != nil {
			continue
		}

		eImg := ebiten.NewImageFromImage(decoded)
		vp.frames = append(vp.frames, &VideoFrame{
			Image: eImg,
			PTS:   pts,
		})
		pts += frameInterval
	}

	if len(vp.frames) == 0 {
		return fmt.Errorf("no valid frames found in %s", dir)
	}
	return nil
}

// Play 开始播放
func (vp *VideoPlayer) Play() {
	vp.mu.Lock()
	defer vp.mu.Unlock()
	vp.playing = true
	vp.current = 0
	vp.elapsed = 0
	vp.finished = false
	if len(vp.frames) > 0 {
		vp.currentFrame = vp.frames[0].Image
	}
}

// Update 更新播放状态
func (vp *VideoPlayer) Update(delta float64) {
	vp.mu.Lock()
	defer vp.mu.Unlock()

	if !vp.playing || vp.finished || len(vp.frames) == 0 {
		return
	}

	vp.elapsed += delta

	// 找到当前应显示的帧
	nextIdx := vp.current
	for nextIdx+1 < len(vp.frames) && vp.frames[nextIdx+1].PTS <= vp.elapsed {
		nextIdx++
	}

	if nextIdx >= len(vp.frames)-1 && vp.elapsed >= vp.frames[len(vp.frames)-1].PTS+1.0/vp.fps {
		vp.finished = true
		vp.playing = false
		return
	}

	vp.current = nextIdx
	vp.currentFrame = vp.frames[vp.current].Image
}

// Draw 渲染当前帧
func (vp *VideoPlayer) Draw(screen *ebiten.Image) {
	vp.mu.Lock()
	defer vp.mu.Unlock()

	if vp.currentFrame == nil {
		return
	}

	op := &ebiten.DrawImageOptions{}
	w := float64(vp.currentFrame.Bounds().Dx())
	h := float64(vp.currentFrame.Bounds().Dy())
	scaleX := float64(vp.screenW) / w
	scaleY := float64(vp.screenH) / h
	scale := scaleX
	if scaleY < scale {
		scale = scaleY
	}

	newW := w * scale
	newH := h * scale
	offsetX := (float64(vp.screenW) - newW) / 2
	offsetY := (float64(vp.screenH) - newH) / 2

	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(offsetX, offsetY)
	screen.DrawImage(vp.currentFrame, op)
}

// Finished 是否播放完毕
func (vp *VideoPlayer) Finished() bool {
	return vp.finished
}

// Stop 停止播放
func (vp *VideoPlayer) Stop() {
	vp.mu.Lock()
	defer vp.mu.Unlock()
	vp.playing = false
	vp.finished = true
}

// sortFileNames 排序文件名
func sortFileNames(files []string) []string {
	sorted := make([]string, len(files))
	copy(sorted, files)
	sort.Strings(sorted)
	return sorted
}
