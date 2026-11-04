package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Settings struct {
	Nickname       string `json:"nickname"`
	Voice          bool   `json:"voice"`
	Volume         int    `json:"volume"`
	SpeechLanguage string `json:"speechLanguage"`
}

type Message struct {
	ID        string   `json:"id"`
	RequestID string   `json:"requestId,omitempty"`
	Role      string   `json:"role"`
	Content   string   `json:"content"`
	Status    string   `json:"status"`
	CreatedAt string   `json:"createdAt"`
	SongIDs   []string `json:"songIds,omitempty"`
}

type Session struct {
	ID          string    `json:"id"`
	CompanionID string    `json:"companionId"`
	Title       string    `json:"title"`
	CreatedAt   string    `json:"createdAt"`
	UpdatedAt   string    `json:"updatedAt"`
	Messages    []Message `json:"messages"`
}

type Companion struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Identity      string `json:"identity"`
	Personality   string `json:"personality"`
	SpeakingStyle string `json:"speakingStyle"`
	Greeting      string `json:"greeting"`
	Theme         string `json:"theme"`
	ImageURL      string `json:"imageUrl"`
	Voice         string `json:"voice"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

func defaultCompanion() Companion {
	return Companion{ID: "default-whale", Name: "大肥鱼", Identity: "蓝色长发、蓝色眼睛、带鲸鱼尾巴的Q版鲸鱼娘伙伴，戴着白色女仆发饰", Personality: "温柔，有一点俏皮，耐心倾听", SpeakingStyle: "简短自然的中文，通常回复1到3句，避免说教和反复自我介绍", Greeting: "嗨，我是大肥鱼。今天的海很安静，刚好可以听你慢慢说。", Theme: "ocean", ImageURL: "/assets/whale-girl-sheet.png", Voice: "sakiko_white"}
}

func anonCompanion() Companion {
	return Companion{
		ID: "template-anon", Name: "千早爱音", Identity: "MyGO!!!!! 的吉他手千早爱音，以角色设定陪你分享日常、音乐与练习中的小事",
		Personality:   "开朗主动，爱漂亮、关心朋友，有一点小骄傲；遇到挫折也会鼓起勇气向前走",
		SpeakingStyle: "轻松自然的中文口语，简短、有活力，偶尔俏皮地打趣，不刻意卖萌，也不反复介绍自己",
		Greeting:      "你来啦！今天想聊些什么？练吉他的事、刚刚发生的小事，我都想听。",
		Theme:         "anon", ImageURL: "/assets/anon.png", Voice: "anon",
	}
}

func findCompanion(s *State, id string) *Companion {
	for i := range s.Companions {
		if s.Companions[i].ID == id {
			return &s.Companions[i]
		}
	}
	return nil
}

func companionForSession(state State, session Session) Companion {
	if companion := findCompanion(&state, session.CompanionID); companion != nil {
		return *companion
	}
	if companion := findCompanion(&state, state.ActiveCompanionID); companion != nil {
		return *companion
	}
	return defaultCompanion()
}

type Memory struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Category  string `json:"category"`
	UpdatedAt string `json:"updatedAt"`
}

type State struct {
	DefaultUIVersion  int         `json:"defaultUIVersion"`
	Version           int         `json:"version"`
	Settings          Settings    `json:"settings"`
	Sessions          []Session   `json:"sessions"`
	Memories          []Memory    `json:"memories"`
	Companions        []Companion `json:"companions"`
	ActiveCompanionID string      `json:"activeCompanionId"`
}

type Store struct {
	mu   sync.Mutex
	path string
	data State
}

func newID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("secure random generator unavailable")
	}
	return hex.EncodeToString(b[:])
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func openStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "state.json"), data: State{
		Version: 1, Settings: Settings{Volume: 70, SpeechLanguage: "zh"},
		Sessions: []Session{}, Memories: []Memory{},
	}}
	data, err := os.ReadFile(s.path)
	if err == nil {
		if err := json.Unmarshal(data, &s.data); err != nil {
			return nil, errors.New("本地数据文件无法读取，请先备份 data/state.json 后检查内容")
		}
		if s.data.Version != 1 {
			return nil, errors.New("本地数据版本不受支持")
		}
		// A pending request cannot survive a process restart.
		for i := range s.data.Sessions {
			for j := range s.data.Sessions[i].Messages {
				if s.data.Sessions[i].Messages[j].Status == "pending" {
					s.data.Sessions[i].Messages[j].Status = "cancelled"
				}
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	// Older v1 files only contained the fixed whale character. Keep their history
	// attached to that character while adding profiles without resetting user data.
	if len(s.data.Companions) == 0 {
		s.data.Companions = []Companion{defaultCompanion()}
	}
	// Bind existing history before adding a new default template. A visual update
	// must not turn an old whale conversation into a different character.
	if findCompanion(&s.data, s.data.ActiveCompanionID) == nil {
		s.data.ActiveCompanionID = s.data.Companions[0].ID
	}
	for i := range s.data.Sessions {
		if findCompanion(&s.data, s.data.Sessions[i].CompanionID) == nil {
			s.data.Sessions[i].CompanionID = s.data.Companions[0].ID
		}
	}
	for i := range s.data.Companions {
		companion := &s.data.Companions[i]
		if companion.Theme == "" {
			companion.Theme = "ocean"
		}
		if companion.ImageURL == "" {
			companion.ImageURL = "/assets/whale-girl-sheet.png"
		}
		if companion.Voice == "" {
			companion.Voice = "default"
			if companion.ID == "default-whale" {
				companion.Voice = "sakiko_white"
			}
			if companion.ID == "template-anon" {
				companion.Voice = "anon"
			}
		}
	}
	if findCompanion(&s.data, "template-anon") == nil {
		s.data.Companions = append(s.data.Companions, anonCompanion())
	}
	// Select the whale UI once for this update, without rewriting old sessions
	// or changing later user selections. Custom names and explicit voices stay.
	if s.data.DefaultUIVersion < 1 {
		whale := findCompanion(&s.data, "default-whale")
		if whale == nil {
			s.data.Companions = append(s.data.Companions, defaultCompanion())
			whale = findCompanion(&s.data, "default-whale")
		}
		if whale.Name == "小鲸" {
			whale.Name = "大肥鱼"
		}
		if whale.Greeting == "嗨，我是小鲸。今天的海很安静，刚好可以听你慢慢说。" {
			whale.Greeting = defaultCompanion().Greeting
		}
		if whale.Voice == "default" || whale.Voice == "" {
			whale.Voice = "sakiko_white"
		}
		anon := findCompanion(&s.data, "template-anon")
		if anon.Voice == "" || anon.Voice == "default" {
			anon.Voice = "anon"
		}
		s.data.ActiveCompanionID = "default-whale"
		s.data.DefaultUIVersion = 1
	}
	if s.data.Settings.SpeechLanguage != "zh" && s.data.Settings.SpeechLanguage != "yue" {
		s.data.Settings.SpeechLanguage = "zh"
	}
	return s, nil
}

func cloneState(s State) State {
	b, _ := json.Marshal(s)
	var result State
	_ = json.Unmarshal(b, &result)
	return result
}

func (s *Store) snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneState(s.data)
}

// Persist a complete replacement before exposing it to readers.
func (s *Store) update(fn func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneState(s.data)
	if err := fn(&next); err != nil {
		return err
	}
	b, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".state-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	_ = f.Chmod(0600)
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.data = next
	return nil
}

func findSession(s *State, id string) *Session {
	for i := range s.Sessions {
		if s.Sessions[i].ID == id {
			return &s.Sessions[i]
		}
	}
	return nil
}
