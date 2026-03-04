package engine

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/mp3"
	//"github.com/hajimehoshi/ebiten/v2/audio/ogg"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"
)

const sampleRate = 44100

// AudioChannel 音频通道
type AudioChannel struct {
	player *audio.Player
	path   string
	loop   bool
	paused bool
}

// AudioSystem 音频系统
type AudioSystem struct {
	context *audio.Context
	fs      *FileSystem
	cache   *Cache
	config  *Config

	bgm    *AudioChannel
	voice  *AudioChannel
	seList []*AudioChannel

	mu               sync.Mutex
	currentVoicePath string
}

// NewAudioSystem 创建音频系统
func NewAudioSystem(fs *FileSystem, cache *Cache, config *Config) (*AudioSystem, error) {
	ctx := audio.NewContext(sampleRate)
	return &AudioSystem{
		context: ctx,
		fs:      fs,
		cache:   cache,
		config:  config,
	}, nil
}

// PlayBGM 播放BGM
func (as *AudioSystem) PlayBGM(path string, loop bool) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	if as.bgm != nil && as.bgm.player != nil {
		as.bgm.player.Close()
	}

	player, err := as.createPlayer(path, loop)
	if err != nil {
		return fmt.Errorf("PlayBGM %s: %w", path, err)
	}

	vol := as.config.BGMVolume * as.config.MasterVolume
	player.SetVolume(vol)
	player.Play()
	as.bgm = &AudioChannel{player: player, path: path, loop: loop}
	return nil
}

// StopBGM 停止BGM
func (as *AudioSystem) StopBGM() {
	as.mu.Lock()
	defer as.mu.Unlock()
	if as.bgm != nil && as.bgm.player != nil {
		as.bgm.player.Close()
		as.bgm = nil
	}
}

// PlayVoice 播放人声
func (as *AudioSystem) PlayVoice(path string) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	if as.voice != nil && as.voice.player != nil {
		as.voice.player.Close()
	}
	as.currentVoicePath = path

	player, err := as.createPlayer(path, false)
	if err != nil {
		return fmt.Errorf("PlayVoice %s: %w", path, err)
	}

	vol := as.config.VoiceVolume * as.config.MasterVolume
	player.SetVolume(vol)
	player.Play()
	as.voice = &AudioChannel{player: player, path: path}
	return nil
}

// StopVoice 停止人声
func (as *AudioSystem) StopVoice() {
	as.mu.Lock()
	defer as.mu.Unlock()
	if as.voice != nil && as.voice.player != nil {
		as.voice.player.Close()
		as.voice = nil
	}
}

// ReplayVoice 重播人声
func (as *AudioSystem) ReplayVoice() {
	if as.currentVoicePath != "" {
		as.PlayVoice(as.currentVoicePath)
	}
}

// PlaySE 播放音效
func (as *AudioSystem) PlaySE(path string, loop bool) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	player, err := as.createPlayer(path, loop)
	if err != nil {
		return fmt.Errorf("PlaySE %s: %w", path, err)
	}

	vol := as.config.SEVolume * as.config.MasterVolume
	player.SetVolume(vol)
	player.Play()
	as.seList = append(as.seList, &AudioChannel{player: player, path: path, loop: loop})
	return nil
}

// createPlayer 创建播放器（不使用 audio.ReadSeekCloser）
func (as *AudioSystem) createPlayer(path string, loop bool) (*audio.Player, error) {
	data, err := as.cache.GetData(path)
	if err != nil {
		return nil, err
	}

	ext := getExtension(path)
	switch ext {
	case "mp3":
		s, err := mp3.DecodeWithSampleRate(sampleRate, bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("mp3 decode: %w", err)
		}
		if loop {
			return as.context.NewPlayer(audio.NewInfiniteLoop(s, s.Length()))
		}
		return as.context.NewPlayer(s)

	//case "ogg":
	//	s, err := ogg.DecodeWithSampleRate(sampleRate, bytes.NewReader(data))
	//	if err != nil {
	//		return nil, fmt.Errorf("ogg decode: %w", err)
	//	}
	//	if loop {
	//		return as.context.NewPlayer(audio.NewInfiniteLoop(s, s.Length()))
	//	}
	//	return as.context.NewPlayer(s)

	case "wav":
		s, err := wav.DecodeWithSampleRate(sampleRate, bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("wav decode: %w", err)
		}
		if loop {
			return as.context.NewPlayer(audio.NewInfiniteLoop(s, s.Length()))
		}
		return as.context.NewPlayer(s)

	default:
		return nil, fmt.Errorf("unsupported audio format: %s", ext)
	}
}

// Update 更新音频状态
func (as *AudioSystem) Update() {
	as.mu.Lock()
	defer as.mu.Unlock()

	active := as.seList[:0]
	for _, ch := range as.seList {
		if ch.player != nil && ch.player.IsPlaying() {
			active = append(active, ch)
		} else if ch.player != nil {
			ch.player.Close()
		}
	}
	as.seList = active
}

// SetMasterVolume 主音量
func (as *AudioSystem) SetMasterVolume(v float64) {
	as.config.MasterVolume = v
	as.updateVolumes()
}

// SetBGMVolume BGM音量
func (as *AudioSystem) SetBGMVolume(v float64) {
	as.config.BGMVolume = v
	if as.bgm != nil && as.bgm.player != nil {
		as.bgm.player.SetVolume(v * as.config.MasterVolume)
	}
}

// SetSEVolume 音效音量
func (as *AudioSystem) SetSEVolume(v float64) {
	as.config.SEVolume = v
	for _, ch := range as.seList {
		if ch.player != nil {
			ch.player.SetVolume(v * as.config.MasterVolume)
		}
	}
}

// SetVoiceVolume 人声音量
func (as *AudioSystem) SetVoiceVolume(v float64) {
	as.config.VoiceVolume = v
	if as.voice != nil && as.voice.player != nil {
		as.voice.player.SetVolume(v * as.config.MasterVolume)
	}
}

func (as *AudioSystem) updateVolumes() {
	if as.bgm != nil && as.bgm.player != nil {
		as.bgm.player.SetVolume(as.config.BGMVolume * as.config.MasterVolume)
	}
	if as.voice != nil && as.voice.player != nil {
		as.voice.player.SetVolume(as.config.VoiceVolume * as.config.MasterVolume)
	}
	for _, ch := range as.seList {
		if ch.player != nil {
			ch.player.SetVolume(as.config.SEVolume * as.config.MasterVolume)
		}
	}
}

// ToggleVoice 切换暂停/继续
func (as *AudioSystem) ToggleVoice() {
	as.mu.Lock()
	defer as.mu.Unlock()
	if as.voice == nil || as.voice.player == nil {
		return
	}
	if as.voice.player.IsPlaying() {
		as.voice.player.Pause()
	} else {
		as.voice.player.Play()
	}
}

// IsVoicePlaying 是否在播放
func (as *AudioSystem) IsVoicePlaying() bool {
	if as.voice != nil && as.voice.player != nil {
		return as.voice.player.IsPlaying()
	}
	return false
}

// StopAllSE 停止所有音效
func (as *AudioSystem) StopAllSE() {
	as.mu.Lock()
	defer as.mu.Unlock()
	for _, ch := range as.seList {
		if ch.player != nil {
			ch.player.Close()
		}
	}
	as.seList = nil
}

// GetCurrentVoicePath 获取当前人声路径
func (as *AudioSystem) GetCurrentVoicePath() string {
	return as.currentVoicePath
}

func getExtension(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return strings.ToLower(path[i+1:])
		}
	}
	return ""
}
