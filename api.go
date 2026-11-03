package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

type App struct {
	store  *Store
	config Config
	client *http.Client
	busy   atomic.Bool
}

type Song struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Duration int    `json:"duration"`
	URL      string `json:"url"`
	Theme    string `json:"theme"`
}

var songs = []Song{
	{"tide", "晚风与海", "让今天的心事，慢慢靠岸", 36, "/assets/tide.wav", "sea"},
	{"moon", "月亮来信", "给还没睡的你，一点温柔", 36, "/assets/moon.wav", "moon"},
	{"cloud", "云朵散步", "没有目的地，也可以很开心", 36, "/assets/cloud.wav", "cloud"},
}

func (a *App) routes() *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true, "app": "whale-demo", "version": "0.1"})
	})
	m.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, a.store.snapshot()) })
	m.HandleFunc("GET /api/config", a.publicConfig)
	m.HandleFunc("POST /api/sessions", a.createSession)
	m.HandleFunc("DELETE /api/sessions/{id}", a.deleteSession)
	m.HandleFunc("DELETE /api/sessions", a.clearSessions)
	m.HandleFunc("POST /api/chat", a.chat)
	m.HandleFunc("PUT /api/settings", a.settings)
	m.HandleFunc("POST /api/memories", a.saveMemory)
	m.HandleFunc("PUT /api/memories/{id}", a.saveMemory)
	m.HandleFunc("DELETE /api/memories/{id}", a.deleteMemory)
	m.HandleFunc("DELETE /api/memories", a.clearMemories)
	m.HandleFunc("POST /api/transcribe", a.transcribe)
	m.HandleFunc("POST /api/speech", a.speech)
	return m
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeError(w, 415, "请求格式不正确，请刷新页面后重试")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	d := json.NewDecoder(r.Body)
	if err := d.Decode(value); err != nil {
		writeError(w, 400, "无法读取提交内容")
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		writeError(w, 400, "提交内容格式不正确")
		return false
	}
	return true
}

func (a *App) publicConfig(w http.ResponseWriter, r *http.Request) {
	mode := "demo"
	if a.config.liveChat() {
		mode = "live"
	}
	writeJSON(w, 200, map[string]any{
		"chatMode": mode, "serverASR": a.config.serverASR(), "serverTTS": a.config.serverTTS(),
		"character": map[string]string{"name": "小鲸", "greeting": "嗨，我是小鲸。今天的海很安静，刚好可以听你慢慢说。"},
		"songs":     songs, "songMode": "synthetic-humming", "maxRecordingSeconds": 60,
	})
}

func (a *App) createSession(w http.ResponseWriter, r *http.Request) {
	session := Session{ID: newID(), Title: "新的小日常", CreatedAt: now(), UpdatedAt: now(), Messages: []Message{}}
	if err := a.store.update(func(s *State) error {
		if len(s.Sessions) >= 100 {
			return errors.New("会话已达到 100 个，请先整理一些历史记录")
		}
		s.Sessions = append([]Session{session}, s.Sessions...)
		return nil
	}); err != nil {
		writeError(w, 400, "暂时无法新建会话，请检查本地空间或整理历史记录")
		return
	}
	writeJSON(w, 201, session)
}

func (a *App) deleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := a.store.update(func(s *State) error {
		for i := range s.Sessions {
			if s.Sessions[i].ID == id {
				s.Sessions = append(s.Sessions[:i], s.Sessions[i+1:]...)
				return nil
			}
		}
		return errors.New("missing")
	})
	if err != nil {
		writeError(w, 404, "未找到这段对话，或暂时无法保存修改")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) clearSessions(w http.ResponseWriter, r *http.Request) {
	if err := a.store.update(func(s *State) error { s.Sessions = []Session{}; return nil }); err != nil {
		writeError(w, 500, "无法清空聊天记录")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) settings(w http.ResponseWriter, r *http.Request) {
	var settings Settings
	if !decode(w, r, &settings) {
		return
	}
	settings.Nickname = strings.TrimSpace(settings.Nickname)
	if utf8.RuneCountInString(settings.Nickname) > 24 || settings.Volume < 0 || settings.Volume > 100 {
		writeError(w, 400, "称呼最多 24 个字，音量应在 0 到 100 之间")
		return
	}
	if err := a.store.update(func(s *State) error { s.Settings = settings; return nil }); err != nil {
		writeError(w, 500, "设置暂时没有保存成功")
		return
	}
	writeJSON(w, 200, settings)
}

func (a *App) saveMemory(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Content  string `json:"content"`
		Category string `json:"category"`
	}
	if !decode(w, r, &input) {
		return
	}
	input.Content, input.Category = strings.TrimSpace(input.Content), strings.TrimSpace(input.Category)
	if input.Content == "" || utf8.RuneCountInString(input.Content) > 300 || utf8.RuneCountInString(input.Category) > 20 {
		writeError(w, 400, "记忆需要有内容，且不超过 300 个字")
		return
	}
	if input.Category == "" {
		input.Category = "小偏好"
	}
	id := r.PathValue("id")
	memory := Memory{ID: id, Content: input.Content, Category: input.Category, UpdatedAt: now()}
	err := a.store.update(func(s *State) error {
		if id != "" {
			for i := range s.Memories {
				if s.Memories[i].ID == id {
					s.Memories[i] = memory
					return nil
				}
			}
			return errors.New("missing")
		}
		if len(s.Memories) >= 100 {
			return errors.New("full")
		}
		memory.ID = newID()
		s.Memories = append(s.Memories, memory)
		return nil
	})
	if err != nil {
		writeError(w, 400, "暂时无法保存记忆，请确认条目仍存在且总数没有超过 100 条")
		return
	}
	writeJSON(w, 200, memory)
}

func (a *App) deleteMemory(w http.ResponseWriter, r *http.Request) {
	err := a.store.update(func(s *State) error {
		for i := range s.Memories {
			if s.Memories[i].ID == r.PathValue("id") {
				s.Memories = append(s.Memories[:i], s.Memories[i+1:]...)
				return nil
			}
		}
		return errors.New("missing")
	})
	if err != nil {
		writeError(w, 404, "未找到这条记忆，或暂时无法保存修改")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) clearMemories(w http.ResponseWriter, r *http.Request) {
	if err := a.store.update(func(s *State) error { s.Memories = []Memory{}; return nil }); err != nil {
		writeError(w, 500, "无法清空记忆")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) chat(w http.ResponseWriter, r *http.Request) {
	var input struct {
		SessionID string `json:"sessionId"`
		Content   string `json:"content"`
		RequestID string `json:"requestId"`
	}
	if !decode(w, r, &input) {
		return
	}
	input.Content = strings.TrimSpace(input.Content)
	if input.Content == "" || utf8.RuneCountInString(input.Content) > 1000 || len(input.RequestID) > 80 || input.RequestID == "" {
		writeError(w, 400, "请输入 1 到 1000 个字，并通过页面重新发送")
		return
	}
	if !a.busy.CompareAndSwap(false, true) {
		writeError(w, 409, "小鲸正在处理上一句话，稍等一下或先取消")
		return
	}
	defer a.busy.Store(false)
	var cached *Message
	err := a.store.update(func(s *State) error {
		session := findSession(s, input.SessionID)
		if session == nil {
			return errors.New("missing")
		}
		for i := range session.Messages {
			msg := &session.Messages[i]
			if msg.RequestID != input.RequestID {
				continue
			}
			if msg.Role == "assistant" {
				copy := *msg
				cached = &copy
				return nil
			}
			if msg.Role == "user" && msg.Content != input.Content {
				return errors.New("conflict")
			}
		}
		for i := range session.Messages {
			msg := &session.Messages[i]
			if msg.RequestID == input.RequestID && msg.Role == "user" {
				msg.Status = "pending"
				return nil
			}
		}
		session.Messages = append(session.Messages, Message{ID: newID(), RequestID: input.RequestID, Role: "user", Content: input.Content, Status: "pending", CreatedAt: now()})
		session.UpdatedAt = now()
		if session.Title == "新的小日常" {
			runes := []rune(input.Content)
			if len(runes) > 18 {
				runes = append(runes[:18], '…')
			}
			session.Title = string(runes)
		}
		return nil
	})
	if err != nil {
		writeError(w, 400, "这段对话无法保存，请新建对话或检查本地数据目录")
		return
	}
	if cached != nil {
		writeJSON(w, 200, cached)
		return
	}
	snapshot := a.store.snapshot()
	session := findSession(&snapshot, input.SessionID)
	if session == nil {
		writeError(w, 404, "这段对话已被删除")
		return
	}
	reply, err := a.reply(r.Context(), snapshot, *session, input.Content)
	status := "complete"
	if err != nil {
		status = "failed"
	}
	if r.Context().Err() != nil {
		status = "cancelled"
	}
	response := Message{ID: newID(), RequestID: input.RequestID, Role: "assistant", Content: reply, Status: "complete", CreatedAt: now()}
	if wantsSong(input.Content) {
		response.SongIDs = matchingSongs(input.Content)
	}
	// Persist cancellation/failure on the existing user message; never insert a stale reply.
	errSave := a.store.update(func(s *State) error {
		current := findSession(s, input.SessionID)
		if current == nil {
			return errors.New("deleted")
		}
		if r.Context().Err() != nil {
			status = "cancelled"
		}
		for i := range current.Messages {
			if current.Messages[i].RequestID == input.RequestID && current.Messages[i].Role == "user" {
				current.Messages[i].Status = status
			}
		}
		if status == "complete" {
			current.Messages = append(current.Messages, response)
		}
		current.UpdatedAt = now()
		return nil
	})
	if status == "cancelled" {
		return
	}
	if errSave != nil {
		writeError(w, 500, "回复暂时无法保存，请检查本地空间")
		return
	}
	if err != nil {
		writeError(w, 502, friendlyProviderError(err))
		return
	}
	writeJSON(w, 200, response)
}

func wantsSong(text string) bool {
	for _, word := range []string{"唱", "点歌", "听歌", "听首", "音乐", "晚风与海", "月亮来信", "云朵散步"} {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func matchingSongs(text string) []string {
	var ids []string
	for _, song := range songs {
		if strings.Contains(text, song.Title) {
			ids = append(ids, song.ID)
		}
	}
	if len(ids) == 0 {
		for _, song := range songs {
			ids = append(ids, song.ID)
		}
	}
	return ids
}

func friendlyProviderError(err error) string {
	if errors.Is(err, errQuota) {
		return "对话服务当前额度不足或请求过多，请稍后重试"
	}
	if errors.Is(err, errAuth) {
		return "对话服务暂时连接不上，请检查本机的服务配置"
	}
	return fmt.Sprint("刚才没有收到服务的回复，你写的话已经保留，可以重试。")
}
