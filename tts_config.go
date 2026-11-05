package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The same profile is read by the Python launcher. Model paths stay on the server.
type SoVITSVoice struct {
	Name            string  `json:"name"`
	Version         string  `json:"version"`
	GPTWeights      string  `json:"gpt_weights"`
	SoVITSWeights   string  `json:"sovits_weights"`
	RefAudioPath    string  `json:"ref_audio_path"`
	PromptText      string  `json:"prompt_text"`
	PromptLang      string  `json:"prompt_lang"`
	TextLang        string  `json:"text_lang"`
	TextSplitMethod string  `json:"text_split_method"`
	SpeedFactor     float64 `json:"speed_factor"`
}

type PublicVoice struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
}

type SoVITSProfile struct {
	DefaultVoice    string                 `json:"default_voice"`
	Voices          map[string]SoVITSVoice `json:"voices"`
	Version         string                 `json:"version"`
	RefAudioPath    string                 `json:"ref_audio_path"`
	PromptText      string                 `json:"prompt_text"`
	PromptLang      string                 `json:"prompt_lang"`
	TextLang        string                 `json:"text_lang"`
	TextSplitMethod string                 `json:"text_split_method"`
	SpeedFactor     float64                `json:"speed_factor"`
}

func (c Config) ttsProvider() string {
	if c.TTSProvider != "" {
		return c.TTSProvider
	}
	if c.TTSBase != "" {
		return "openai"
	}
	return "browser"
}

func (c Config) speechTimeout() time.Duration {
	if c.TTSTimeout > 0 {
		return c.TTSTimeout
	}
	if c.ttsProvider() == "gpt-sovits" {
		return 180 * time.Second
	}
	return 45 * time.Second
}

func localVoiceID(value string) bool {
	return value == "anon" || value == "soyo" || value == "sakiko_white"
}

func validVoiceChoice(value string) bool {
	return value == "default" || value == "browser" || localVoiceID(value)
}

func (p SoVITSProfile) defaultVoiceID() string {
	if p.DefaultVoice != "" {
		return p.DefaultVoice
	}
	return "anon" // single-voice configurations from earlier versions
}

func (p SoVITSProfile) voiceProfile(id string) (SoVITSProfile, bool) {
	if id == "" || id == "default" {
		id = p.defaultVoiceID()
	}
	if !localVoiceID(id) {
		return SoVITSProfile{}, false
	}
	if len(p.Voices) == 0 {
		// A legacy profile contains only Anon. Never label it as another voice.
		if id != "anon" {
			return SoVITSProfile{}, false
		}
		return p, true
	}
	v, ok := p.Voices[id]
	if !ok || strings.TrimSpace(v.GPTWeights) == "" || strings.TrimSpace(v.SoVITSWeights) == "" {
		return SoVITSProfile{}, false
	}
	result := p
	result.Voices = nil
	result.DefaultVoice = id
	// Reference audio and transcript must belong to this voice, not the last
	// selected voice or a different top-level profile.
	result.RefAudioPath, result.PromptText = v.RefAudioPath, v.PromptText
	if v.Version != "" {
		result.Version = v.Version
	}
	if v.PromptLang != "" {
		result.PromptLang = v.PromptLang
	}
	if v.TextLang != "" {
		result.TextLang = v.TextLang
	}
	if v.TextSplitMethod != "" {
		result.TextSplitMethod = v.TextSplitMethod
	}
	if v.SpeedFactor != 0 {
		result.SpeedFactor = v.SpeedFactor
	}
	return result, true
}

func (c Config) voiceAvailable(id string) bool {
	p, ok := c.GPTSoVITSProfile.voiceProfile(id)
	return ok && c.GPTSoVITSBase != "" && strings.TrimSpace(p.RefAudioPath) != "" && p.PromptLang != ""
}

func (c Config) anonVoiceAvailable() bool { return c.voiceAvailable("anon") }

func (c Config) publicVoices() []PublicVoice {
	voices := []PublicVoice{{ID: "anon", Name: "千早爱音"}, {ID: "soyo", Name: "长崎素世"}, {ID: "sakiko_white", Name: "客服小祥"}}
	for i := range voices {
		if v, ok := c.GPTSoVITSProfile.Voices[voices[i].ID]; ok && strings.TrimSpace(v.Name) != "" {
			voices[i].Name = v.Name
		}
		voices[i].Available = c.voiceAvailable(voices[i].ID)
	}
	return voices
}

func normalizedSpeechLanguage(value string) string {
	if value == "" {
		return "zh"
	}
	return value
}

func validSpeechLanguage(value string) bool { return value == "zh" || value == "yue" }

func configureTTS(c *Config) error {
	c.TTSProvider = strings.ToLower(env("TTS_PROVIDER", ""))
	switch c.ttsProvider() {
	case "browser", "openai", "gpt-sovits":
	default:
		return fmt.Errorf("TTS_PROVIDER 只能是 browser、openai 或 gpt-sovits")
	}
	seconds, err := strconv.Atoi(env("TTS_TIMEOUT_SECONDS", "180"))
	if err != nil || seconds < 10 || seconds > 600 {
		return fmt.Errorf("TTS_TIMEOUT_SECONDS 应在 10 到 600 秒之间")
	}
	c.TTSTimeout = time.Duration(seconds) * time.Second
	// Keep the Anon template available even when other companions use the
	// device or another provider. An optional missing profile never prevents
	// those providers from starting; selecting Anon explains the missing setup.
	err = loadSoVITSConfig(c)
	if err != nil && c.ttsProvider() == "gpt-sovits" {
		return err
	}
	return nil
}

func loadSoVITSConfig(c *Config) error {
	c.GPTSoVITSProfile = SoVITSProfile{}
	c.GPTSoVITSBase = strings.TrimRight(env("GPT_SOVITS_API_BASE", "http://127.0.0.1:9880"), "/")
	u, err := url.Parse(c.GPTSoVITSBase)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" {
		c.GPTSoVITSBase = ""
		return fmt.Errorf("GPT_SOVITS_API_BASE 不是有效的 HTTP 服务地址")
	}
	path := env("GPT_SOVITS_PROFILE", "config/sovits.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("无法读取 GPT-SoVITS 配置 %s：%w", path, err)
	}
	var p SoVITSProfile
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("GPT-SoVITS 配置不是有效 JSON：%w", err)
	}
	if p.PromptLang == "" || p.TextLang == "" {
		return fmt.Errorf("GPT-SoVITS 配置必须填写 prompt_lang 和 text_lang")
	}
	if p.TextSplitMethod == "" {
		p.TextSplitMethod = "cut5"
	}
	if p.SpeedFactor == 0 {
		p.SpeedFactor = 1
	}
	if p.SpeedFactor < 0.5 || p.SpeedFactor > 2 {
		return fmt.Errorf("GPT-SoVITS speed_factor 应在 0.5 到 2 之间")
	}
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("无法定位 whale-demo：%w", err)
	}
	if strings.TrimSpace(p.RefAudioPath) != "" {
		p.RefAudioPath, err = resolveDemoResource(p.RefAudioPath, root)
		if err != nil {
			return fmt.Errorf("GPT-SoVITS 参考音频路径无效：%w", err)
		}
	}
	if p.DefaultVoice != "" && !localVoiceID(p.DefaultVoice) {
		return fmt.Errorf("default_voice 不是有效的本地音色")
	}
	for id, voice := range p.Voices {
		if !localVoiceID(id) {
			return fmt.Errorf("未知的本地音色：%s", id)
		}
		if voice.SpeedFactor != 0 && (voice.SpeedFactor < 0.5 || voice.SpeedFactor > 2) {
			return fmt.Errorf("%s 的 speed_factor 应在 0.5 到 2 之间", id)
		}
		if strings.TrimSpace(voice.RefAudioPath) != "" {
			voice.RefAudioPath, err = resolveDemoResource(voice.RefAudioPath, root)
			if err != nil {
				return fmt.Errorf("%s 参考音频路径无效：%w", id, err)
			}
		}
		p.Voices[id] = voice
	}
	if len(p.Voices) > 0 {
		if _, ok := p.Voices[p.defaultVoiceID()]; !ok {
			return fmt.Errorf("default_voice 未在 voices 中配置")
		}
	}
	c.GPTSoVITSProfile = p
	return nil
}

// Resolve reference audio relative to the demo, including existing junctions or
// symlinks. Relocating the complete demo must not retain an original drive path.
func resolveDemoResource(value, root string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	path := filepath.Clean(value)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// Evaluate the nearest existing ancestor too, so an absent file underneath
	// a junction cannot disguise a dependency on an external directory.
	ancestor := path
	var suffix []string
	for {
		resolved, evalErr := filepath.EvalSymlinks(ancestor)
		if evalErr == nil {
			path = resolved
			for i := len(suffix) - 1; i >= 0; i-- {
				path = filepath.Join(path, suffix[i])
			}
			break
		}
		if !os.IsNotExist(evalErr) || filepath.Dir(ancestor) == ancestor {
			return "", evalErr
		}
		suffix = append(suffix, filepath.Base(ancestor))
		ancestor = filepath.Dir(ancestor)
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("必须位于 whale-demo 文件夹内")
	}
	return path, nil
}
