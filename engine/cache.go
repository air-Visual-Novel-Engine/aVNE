package engine

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// CacheEntry 缓存条目
type CacheEntry struct {
	data     interface{}
	lastUsed time.Time
	size     int64
}

// Cache 资源缓存系统
type Cache struct {
	fs      *FileSystem
	images  map[string]*CacheEntry
	data    map[string]*CacheEntry
	mu      sync.RWMutex
	maxSize int64 // 最大缓存大小（字节）
	curSize int64
}

// NewCache 创建缓存
func NewCache(fs *FileSystem) *Cache {
	c := &Cache{
		fs:      fs,
		images:  make(map[string]*CacheEntry),
		data:    make(map[string]*CacheEntry),
		maxSize: 512 * 1024 * 1024, // 512MB
	}
	// 定期清理
	go c.cleanupRoutine()
	return c
}

// GetImage 获取图片（带缓存）
func (c *Cache) GetImage(path string) (*ebiten.Image, error) {
	c.mu.RLock()
	if entry, ok := c.images[path]; ok {
		entry.lastUsed = time.Now()
		img := entry.data.(*ebiten.Image)
		c.mu.RUnlock()
		return img, nil
	}
	c.mu.RUnlock()

	// 读取文件
	data, err := c.fs.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// 解码图片
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	ebitenImg := ebiten.NewImageFromImage(img)

	c.mu.Lock()
	c.images[path] = &CacheEntry{
		data:     ebitenImg,
		lastUsed: time.Now(),
		size:     int64(len(data)),
	}
	c.curSize += int64(len(data))
	c.mu.Unlock()

	// 检查是否需要清理
	if c.curSize > c.maxSize {
		go c.cleanup()
	}

	return ebitenImg, nil
}

// GetData 获取数据（带缓存）
func (c *Cache) GetData(path string) ([]byte, error) {
	c.mu.RLock()
	if entry, ok := c.data[path]; ok {
		entry.lastUsed = time.Now()
		d := entry.data.([]byte)
		c.mu.RUnlock()
		return d, nil
	}
	c.mu.RUnlock()

	data, err := c.fs.ReadFile(path)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.data[path] = &CacheEntry{
		data:     data,
		lastUsed: time.Now(),
		size:     int64(len(data)),
	}
	c.curSize += int64(len(data))
	c.mu.Unlock()

	return data, nil
}

// Invalidate 使缓存失效
func (c *Cache) Invalidate(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.images[path]; ok {
		c.curSize -= e.size
		delete(c.images, path)
	}
	if e, ok := c.data[path]; ok {
		c.curSize -= e.size
		delete(c.data, path)
	}
}

// cleanup 清理最久未使用的缓存
func (c *Cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	target := c.maxSize * 3 / 4
	for c.curSize > target {
		var oldest string
		var oldestTime time.Time
		for k, v := range c.images {
			if oldest == "" || v.lastUsed.Before(oldestTime) {
				oldest = k
				oldestTime = v.lastUsed
			}
		}
		if oldest == "" {
			break
		}
		c.curSize -= c.images[oldest].size
		delete(c.images, oldest)
	}
}

func (c *Cache) cleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		if c.curSize > c.maxSize/2 {
			c.cleanup()
		}
	}
}
