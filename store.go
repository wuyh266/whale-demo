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
	Nickname string `json:"nickname"`
	Voice    bool   `json:"voice"`
	Volume   int    `json:"volume"`
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
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt string    `json:"createdAt"`
	UpdatedAt string    `json:"updatedAt"`
	Messages  []Message `json:"messages"`
}

type Memory struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Category  string `json:"category"`
	UpdatedAt string `json:"updatedAt"`
}

type State struct {
	Version  int       `json:"version"`
	Settings Settings  `json:"settings"`
	Sessions []Session `json:"sessions"`
	Memories []Memory  `json:"memories"`
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
		Version: 1, Settings: Settings{Volume: 70},
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
