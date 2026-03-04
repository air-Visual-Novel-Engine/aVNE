package engine

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileSystem 统一文件系统
type FileSystem struct {
	zipPath    string
	basePath   string
	zipReader  *zip.ReadCloser
	zipEntries map[string]*zip.File
	mu         sync.RWMutex
	password   []byte // 简单XOR加密密钥
}

// NewFileSystem 创建文件系统
func NewFileSystem(zipPath, basePath string) (*FileSystem, error) {
	fs := &FileSystem{
		zipPath:    zipPath,
		basePath:   basePath,
		zipEntries: make(map[string]*zip.File),
	}

	if zipPath != "" {
		if _, err := os.Stat(zipPath); err == nil {
			reader, err := zip.OpenReader(zipPath)
			if err != nil {
				return nil, fmt.Errorf("open zip: %w", err)
			}
			fs.zipReader = reader
			for _, f := range reader.File {
				// 统一路径分隔符
				name := filepath.ToSlash(f.Name)
				fs.zipEntries[name] = f
			}
		}
	}

	return fs, nil
}

// ReadFile 读取文件内容
func (fs *FileSystem) ReadFile(path string) ([]byte, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	// 统一路径
	normalPath := filepath.ToSlash(path)

	// 优先从zip读取
	if fs.zipReader != nil {
		if entry, ok := fs.zipEntries[normalPath]; ok {
			rc, err := entry.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, err
			}
			// 如果有密钥，解密
			if len(fs.password) > 0 {
				for i, b := range data {
					data[i] = b ^ fs.password[i%len(fs.password)]
				}
			}
			return data, nil
		}
	}

	// 从本地目录读取
	localPath := filepath.Join(fs.basePath, path)
	data, err := os.ReadFile(localPath)
	if err != nil {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	return data, nil
}

// ReadFileAsReader 获取文件读取器
func (fs *FileSystem) ReadFileAsReader(path string) (io.ReadCloser, error) {
	data, err := fs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(strings.NewReader(string(data))), nil
}

// ReadFileBytes 读取文件为字节（用于二进制文件）
func (fs *FileSystem) ReadFileBytes(path string) (io.ReadCloser, int64, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	normalPath := filepath.ToSlash(path)

	if fs.zipReader != nil {
		if entry, ok := fs.zipEntries[normalPath]; ok {
			rc, err := entry.Open()
			if err != nil {
				return nil, 0, err
			}
			return rc, int64(entry.UncompressedSize64), nil
		}
	}

	localPath := filepath.Join(fs.basePath, path)
	f, err := os.Open(localPath)
	if err != nil {
		return nil, 0, err
	}
	info, _ := f.Stat()
	return f, info.Size(), nil
}

// Exists 检查文件是否存在
func (fs *FileSystem) Exists(path string) bool {
	normalPath := filepath.ToSlash(path)

	if fs.zipReader != nil {
		if _, ok := fs.zipEntries[normalPath]; ok {
			return true
		}
	}

	localPath := filepath.Join(fs.basePath, path)
	_, err := os.Stat(localPath)
	return err == nil
}

// ListDir 列出目录内容
func (fs *FileSystem) ListDir(dir string) ([]string, error) {
	dirPath := filepath.ToSlash(dir)
	if !strings.HasSuffix(dirPath, "/") {
		dirPath += "/"
	}

	seen := make(map[string]bool)
	var files []string

	// 从zip中查找
	if fs.zipReader != nil {
		for name := range fs.zipEntries {
			if strings.HasPrefix(name, dirPath) {
				rest := name[len(dirPath):]
				if rest != "" {
					// 只获取直接子项
					parts := strings.SplitN(rest, "/", 2)
					if !seen[parts[0]] {
						seen[parts[0]] = true
						files = append(files, parts[0])
					}
				}
			}
		}
	}

	// 从本地目录查找
	localDir := filepath.Join(fs.basePath, dir)
	entries, err := os.ReadDir(localDir)
	if err == nil {
		for _, e := range entries {
			if !seen[e.Name()] {
				seen[e.Name()] = true
				files = append(files, e.Name())
			}
		}
	}

	return files, nil
}

// WriteFile 写入文件（只写到本地）
func (fs *FileSystem) WriteFile(path string, data []byte) error {
	localPath := filepath.Join(fs.basePath, path)
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(localPath, data, 0644)
}

// EnsureDir 确保目录存在
func (fs *FileSystem) EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// Close 关闭文件系统
func (fs *FileSystem) Close() {
	if fs.zipReader != nil {
		fs.zipReader.Close()
	}
}
