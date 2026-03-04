package engine

import (
	"fmt"
	"strings"
)

// ScriptLine 脚本行类型
type ScriptLineType int

const (
	LineText ScriptLineType = iota
	LineCommand
	LineLabel
	LineChoice
	LineEmpty
)

// ScriptLine 脚本行
type ScriptLine struct {
	Type      ScriptLineType
	Raw       string
	Speaker   string
	Text      string
	Command   string
	Args      map[string]string
	LabelName string
	IsRead    bool
}

// Choice 选择支
type Choice struct {
	Text      string
	Target    string // 跳转标签
	Affinity  string // 好感度变量名
	AffinityV int    // 好感度变化值
}

// ScriptEngine 脚本引擎
type ScriptEngine struct {
	eng    *Engine
	lines  []ScriptLine
	cursor int

	// 选择支
	choices    []Choice
	hasChoices bool

	// 变量系统
	vars    map[string]int
	strVars map[string]string

	// 好感度
	affinity map[string]int

	// 当前脚本文件
	currentFile string

	// 等待状态
	waiting   bool
	waitTimer float64

	// 已读行记录
	readLines map[string]bool

	// 跳转栈
	callStack []int
}

// NewScriptEngine 创建脚本引擎
func NewScriptEngine(eng *Engine) (*ScriptEngine, error) {
	se := &ScriptEngine{
		eng:       eng,
		vars:      make(map[string]int),
		strVars:   make(map[string]string),
		affinity:  make(map[string]int),
		readLines: make(map[string]bool),
	}
	return se, nil
}

// LoadScript 加载脚本文件
func (se *ScriptEngine) LoadScript(path string) error {
	data, err := se.eng.fs.ReadFile(path)
	if err != nil {
		return fmt.Errorf("load script %s: %w", path, err)
	}

	se.currentFile = path
	se.lines = parseKrkrScript(string(data))
	se.cursor = 0
	se.choices = nil
	se.hasChoices = false
	se.waiting = false

	// 执行到第一个文字行
	se.runUntilDisplay()
	return nil
}

// parseKrkrScript 解析KrkrR风格脚本
func parseKrkrScript(src string) []ScriptLine {
	var lines []ScriptLine
	rawLines := strings.Split(src, "\n")

	for _, raw := range rawLines {
		raw = strings.TrimRight(raw, "\r")
		line := parseSingleLine(raw)
		lines = append(lines, line)
	}
	return lines
}

func parseSingleLine(raw string) ScriptLine {
	trimmed := strings.TrimSpace(raw)

	if trimmed == "" {
		return ScriptLine{Type: LineEmpty, Raw: raw}
	}

	// 注释
	if strings.HasPrefix(trimmed, ";") {
		return ScriptLine{Type: LineEmpty, Raw: raw}
	}

	// 标签 *labelname
	if strings.HasPrefix(trimmed, "*") {
		return ScriptLine{
			Type:      LineLabel,
			Raw:       raw,
			LabelName: strings.TrimPrefix(trimmed, "*"),
		}
	}

	// 命令行 @command ...
	if strings.HasPrefix(trimmed, "@") {
		return parseCommandLine(trimmed[1:], raw)
	}

	// 命令行 [command ...] 单行
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		inner := trimmed[1 : len(trimmed)-1]
		return parseCommandLine(inner, raw)
	}

	// 文字行（可能包含说话者前缀 "名前「文字」"）
	speaker, text := parseSpeakerText(trimmed)
	return ScriptLine{
		Type:    LineText,
		Raw:     raw,
		Speaker: speaker,
		Text:    text,
	}
}

func parseCommandLine(cmdStr, raw string) ScriptLine {
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return ScriptLine{Type: LineEmpty, Raw: raw}
	}

	cmd := parts[0]
	args := make(map[string]string)
	for _, p := range parts[1:] {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			args[kv[0]] = strings.Trim(kv[1], "\"'")
		} else {
			args[kv[0]] = "true"
		}
	}

	return ScriptLine{
		Type:    LineCommand,
		Raw:     raw,
		Command: strings.ToLower(cmd),
		Args:    args,
	}
}

func parseSpeakerText(line string) (speaker, text string) {
	// 格式：「名前」文字内容 或 名前「文字内容
	if idx := strings.Index(line, "「"); idx >= 0 {
		speaker = line[:idx]
		text = line[idx+len("「"):]
		text = strings.TrimSuffix(text, "」")
		return
	}
	// 格式：名前：文字内容
	if idx := strings.Index(line, "："); idx >= 0 {
		speaker = line[:idx]
		text = line[idx+len("："):]
		return
	}
	return "", line
}

// Advance 推进脚本（点击下一行）
func (se *ScriptEngine) Advance() {
	if se.hasChoices {
		return
	}

	if !se.eng.textSys.IsFinished() {
		se.eng.textSys.SkipAll()
		return
	}

	// 停止人声（根据配置）
	if se.eng.config.VoiceStopOnClick {
		se.eng.audio.StopVoice()
	}

	se.cursor++
	se.runUntilDisplay()
}

// runUntilDisplay 执行到下一个需要显示的行
func (se *ScriptEngine) runUntilDisplay() {
	for se.cursor < len(se.lines) {
		line := se.lines[se.cursor]
		switch line.Type {
		case LineEmpty:
			se.cursor++
			continue
		case LineLabel:
			se.cursor++
			continue
		case LineText:
			se.executeText(line)
			return
		case LineCommand:
			if done := se.executeCommand(line); !done {
				return
			}
			se.cursor++
			continue
		}
		se.cursor++
	}
}

func (se *ScriptEngine) executeText(line ScriptLine) {
	// 记录为已读
	readKey := fmt.Sprintf("%s:%d", se.currentFile, se.cursor)
	se.readLines[readKey] = true

	// 解析文字样式
	tl := ParseLine(line.Text, DefaultTextStyle())
	tl.SpeakerName = line.Speaker

	// 设置文字系统
	se.eng.textSys.FadeOut(func() {
		se.eng.textSys.SetLine(tl)
	})
	se.eng.textSys.SetLine(tl)

	// 触发鉴赏系统记录
	se.eng.gallery.RecordEvent(se.currentFile, se.cursor)
}

func (se *ScriptEngine) executeCommand(line ScriptLine) bool {
	cmd := line.Command
	args := line.Args

	switch cmd {
	case "bg":
		// @bg file=xxx layer=0 x=0 y=0
		file := args["file"]
		layerIdx := parseInt(args["layer"], 0)
		x := parseFloat(args["x"], 0)
		y := parseFloat(args["y"], 0)
		maskFile := args["mask"]

		if err := se.eng.layers.SetLayerImage(layerIdx, file, x, y); err != nil {
			fmt.Printf("bg error: %v\n", err)
		}
		if maskFile != "" {
			se.eng.layers.SetLayerMask(layerIdx, maskFile)
		}
		// 记录CG
		if file != "" {
			se.eng.gallery.RecordCG(file)
		}

	case "ch", "character":
		// @ch name=xxx file=xxx layer=5 x=100 y=0 scale=1.0
		file := args["file"]
		layerIdx := parseInt(args["layer"], 5)
		x := parseFloat(args["x"], 0)
		y := parseFloat(args["y"], 0)
		if err := se.eng.layers.SetLayerImage(layerIdx, file, x, y); err != nil {
			fmt.Printf("ch error: %v\n", err)
		}
		scaleS := parseFloat(args["scale"], 1.0)
		if scaleS != 1.0 {
			se.eng.layers.GetLayer(layerIdx).ScaleX = scaleS
			se.eng.layers.GetLayer(layerIdx).ScaleY = scaleS
		}

	case "move":
		// @move layer=0 x=100 y=0 time=1.0 easing=inout
		layerIdx := parseInt(args["layer"], 0)
		x := parseFloat(args["x"], 0)
		y := parseFloat(args["y"], 0)
		duration := parseFloat(args["time"], 1.0)
		easing := parseEasing(args["easing"])
		se.eng.layers.MoveLayer(layerIdx, x, y, duration, easing)

	case "scale":
		// @scale layer=0 sx=1.5 sy=1.5 time=1.0 easing=inout
		layerIdx := parseInt(args["layer"], 0)
		sx := parseFloat(args["sx"], 1.0)
		sy := parseFloat(args["sy"], 1.0)
		duration := parseFloat(args["time"], 1.0)
		easing := parseEasing(args["easing"])
		se.eng.layers.ScaleLayer(layerIdx, sx, sy, duration, easing)

	case "fade":
		// @fade layer=0 alpha=0.0 time=1.0
		layerIdx := parseInt(args["layer"], 0)
		alpha := parseFloat(args["alpha"], 0)
		duration := parseFloat(args["time"], 1.0)
		easing := parseEasing(args["easing"])
		se.eng.layers.FadeLayer(layerIdx, alpha, duration, easing)

	case "wait":
		// @wait time=1.0
		duration := parseFloat(args["time"], 1.0)
		se.waiting = true
		se.waitTimer = duration
		return false // 不继续

	case "waitanim":
		// 等待所有动画完成
		if se.eng.layers.IsAnimating() {
			return false
		}

	case "playse", "se":
		// @se file=xxx
		file := args["file"]
		loop := args["loop"] == "true"
		se.eng.audio.PlaySE(file, loop)

	case "playbgm", "bgm":
		// @bgm file=xxx loop=true
		file := args["file"]
		loop := args["loop"] != "false"
		se.eng.audio.PlayBGM(file, loop)

	case "stopbgm":
		se.eng.audio.StopBGM()

	case "voice":
		// @voice file=xxx
		file := args["file"]
		se.eng.audio.PlayVoice(file)
		if se.eng.textSys.currentLine != nil {
			se.eng.textSys.currentLine.VoicePath = file
		}

	case "video":
		// @video file=xxx
		file := args["file"]
		if err := se.eng.video.Load(file); err != nil {
			fmt.Printf("video error: %v\n", err)
		} else {
			se.eng.video.Play()
			se.eng.SetState(StateVideo)
			return false
		}

	case "jump":
		// @jump target=*label
		target := args["target"]
		if err := se.jumpToLabel(target); err != nil {
			fmt.Printf("jump error: %v\n", err)
		}
		return true

	case "call":
		// @call target=*label (带返回)
		target := args["target"]
		se.callStack = append(se.callStack, se.cursor+1)
		if err := se.jumpToLabel(target); err != nil {
			fmt.Printf("call error: %v\n", err)
		}
		return true

	case "return":
		if len(se.callStack) > 0 {
			se.cursor = se.callStack[len(se.callStack)-1]
			se.callStack = se.callStack[:len(se.callStack)-1]
			return true
		}

	case "choice":
		// @choice
		// 后续行是选项
		se.loadChoices()
		return false

	case "if":
		// @if var=xxx val=yyy target=*label
		varName := args["var"]
		val := parseInt(args["val"], 0)
		target := args["target"]
		op := args["op"] // eq, ne, gt, lt, ge, le
		varVal := se.vars[varName]

		cond := false
		switch op {
		case "eq", "":
			cond = varVal == val
		case "ne":
			cond = varVal != val
		case "gt":
			cond = varVal > val
		case "lt":
			cond = varVal < val
		case "ge":
			cond = varVal >= val
		case "le":
			cond = varVal <= val
		}

		if cond {
			se.jumpToLabel(target)
			return true
		}

	case "setvar":
		// @setvar name=xxx val=yyy
		name := args["name"]
		val := parseInt(args["val"], 0)
		op := args["op"] // set, add, sub
		switch op {
		case "add":
			se.vars[name] += val
		case "sub":
			se.vars[name] -= val
		default:
			se.vars[name] = val
		}

	case "affinity":
		// @affinity name=xxx val=5
		name := args["name"]
		val := parseInt(args["val"], 0)
		se.affinity[name] += val
		fmt.Printf("[Affinity] %s: %d\n", name, se.affinity[name])

	case "clearlay":
		layerIdx := parseInt(args["layer"], -1)
		if layerIdx >= 0 {
			se.eng.layers.ClearLayer(layerIdx)
		}

	case "end":
		se.cursor = len(se.lines)
		return false

	case "title":
		se.eng.StartTransition(StateTitle, false)
		return false

	case "cg":
		// @cg file=xxx folder=xxx 记录CG到鉴赏
		file := args["file"]
		folder := args["folder"]
		se.eng.gallery.RecordCGWithFolder(file, folder)
	}

	return true
}

func (se *ScriptEngine) loadChoices() {
	se.choices = nil
	i := se.cursor + 1
	for i < len(se.lines) {
		l := se.lines[i]
		if l.Type == LineCommand && l.Command == "option" {
			c := Choice{
				Text:      l.Args["text"],
				Target:    l.Args["target"],
				Affinity:  l.Args["affinity"],
				AffinityV: parseInt(l.Args["aval"], 0),
			}
			se.choices = append(se.choices, c)
			i++
		} else if l.Type == LineEmpty {
			i++
		} else {
			break
		}
	}
	se.cursor = i - 1
	se.hasChoices = true
	se.eng.luaBridge.CallFunction("choice_show", se.choicesToLua())
}

func (se *ScriptEngine) choicesToLua() []interface{} {
	var result []interface{}
	for _, c := range se.choices {
		m := map[string]interface{}{
			"text":     c.Text,
			"target":   c.Target,
			"affinity": c.Affinity,
			"aval":     c.AffinityV,
		}
		result = append(result, m)
	}
	return result
}

// SelectChoice 选择选项
func (se *ScriptEngine) SelectChoice(idx int) {
	if idx < 0 || idx >= len(se.choices) {
		return
	}
	c := se.choices[idx]

	// 更新好感度
	if c.Affinity != "" {
		se.affinity[c.Affinity] += c.AffinityV
	}

	// 跳转
	if c.Target != "" {
		se.jumpToLabel(c.Target)
	} else {
		se.cursor++
	}

	se.choices = nil
	se.hasChoices = false

	// 根据配置解除自动/快进
	if se.eng.config.ChoiceUnsetAuto {
		se.eng.autoPlay = false
	}
	if se.eng.config.ChoiceUnsetSkip {
		se.eng.skipMode = false
	}

	se.runUntilDisplay()
}

func (se *ScriptEngine) jumpToLabel(label string) error {
	label = strings.TrimPrefix(label, "*")
	for i, l := range se.lines {
		if l.Type == LineLabel && l.LabelName == label {
			se.cursor = i + 1
			return nil
		}
	}
	return fmt.Errorf("label not found: %s", label)
}

// HasChoices 是否有选择支
func (se *ScriptEngine) HasChoices() bool {
	return se.hasChoices
}

// Update 更新脚本（处理等待等）
func (se *ScriptEngine) Update(delta float64) {
	if se.waiting {
		se.waitTimer -= delta
		if se.waitTimer <= 0 {
			se.waiting = false
			se.cursor++
			se.runUntilDisplay()
		}
	}
}

// GetVar 获取变量
func (se *ScriptEngine) GetVar(name string) int {
	return se.vars[name]
}

// GetAffinity 获取好感度
func (se *ScriptEngine) GetAffinity(name string) int {
	return se.affinity[name]
}

// GetCursor 获取当前行号
func (se *ScriptEngine) GetCursor() int {
	return se.cursor
}

// SetCursor 设置行号（用于读档）
func (se *ScriptEngine) SetCursor(c int) {
	se.cursor = c
}

// 辅助函数
func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	v := def
	fmt.Sscanf(s, "%d", &v)
	return v
}

func parseFloat(s string, def float64) float64 {
	if s == "" {
		return def
	}
	v := def
	fmt.Sscanf(s, "%f", &v)
	return v
}

func parseEasing(s string) EasingType {
	switch strings.ToLower(s) {
	case "inout":
		return EaseInOut
	case "in":
		return EaseIn
	case "out":
		return EaseOut
	default:
		return EaseLinear
	}
}
