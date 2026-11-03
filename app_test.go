package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func testApp(t *testing.T, cfg Config) *App {
	t.Helper()
	store, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &App{store: store, config: cfg, client: &http.Client{Timeout: 2 * time.Second}}
}

func call(t *testing.T, a *App, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buffer bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buffer).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buffer)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	security(a.routes()).ServeHTTP(recorder, req)
	return recorder
}

func newTestSession(t *testing.T, a *App) Session {
	t.Helper()
	response := call(t, a, "POST", "/api/sessions", nil)
	if response.Code != 201 {
		t.Fatalf("create session: %d %s", response.Code, response.Body.String())
	}
	var session Session
	if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	return session
}

func TestStorePersistsAndSnapshotsAreIsolated(t *testing.T) {
	dir := t.TempDir()
	store, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.update(func(s *State) error {
		s.Settings.Nickname = "小林"
		s.Memories = append(s.Memories, Memory{ID: "m1", Content: "喜欢轻音乐"})
		s.Sessions = append(s.Sessions, Session{ID: "s1", Messages: []Message{{ID: "u1", Role: "user", Status: "pending"}}})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// A second write exercises replacing the existing file on Windows.
	if err := store.update(func(s *State) error { s.Settings.Volume = 41; return nil }); err != nil {
		t.Fatal(err)
	}
	copy := store.snapshot()
	copy.Memories[0].Content = "changed outside store"
	if store.snapshot().Memories[0].Content != "喜欢轻音乐" {
		t.Fatal("snapshot mutation leaked")
	}
	reopened, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	state := reopened.snapshot()
	if state.Settings.Nickname != "小林" || state.Settings.Volume != 41 || len(state.Memories) != 1 {
		t.Fatal("persistent data lost")
	}
	if state.Sessions[0].Messages[0].Status != "cancelled" {
		t.Fatal("interrupted request remained pending after restart")
	}
}

func TestChatIdempotencyContextAndMemoryDeletion(t *testing.T) {
	var mu sync.Mutex
	var received [][]chatLine
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected provider route: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("missing server-side credential")
		}
		var body struct {
			Messages []chatLine `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("bad provider payload")
		}
		mu.Lock()
		received = append(received, body.Messages)
		mu.Unlock()
		writeJSON(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "我在听。"}}}})
	}))
	defer provider.Close()
	a := testApp(t, Config{ChatBase: provider.URL, ChatKey: "test-secret", ChatModel: "test-model"})
	first := newTestSession(t, a)
	memory := call(t, a, "POST", "/api/memories", map[string]string{"content": "只喜欢安静的钢琴", "category": "小偏好"})
	if memory.Code != 200 {
		t.Fatal(memory.Body.String())
	}
	var saved Memory
	_ = json.Unmarshal(memory.Body.Bytes(), &saved)
	body := map[string]string{"sessionId": first.ID, "content": "第一段私有话题", "requestId": "request-one"}
	for range 2 {
		if response := call(t, a, "POST", "/api/chat", body); response.Code != 200 {
			t.Fatal(response.Body.String())
		}
	}
	mu.Lock()
	count := len(received)
	firstPrompt := received[0]
	mu.Unlock()
	if count != 1 {
		t.Fatalf("duplicate request reached provider %d times", count)
	}
	if !strings.Contains(firstPrompt[0].Content, "只喜欢安静的钢琴") {
		t.Fatal("confirmed memory not injected")
	}
	if len(a.store.snapshot().Sessions[0].Messages) != 2 {
		t.Fatal("idempotency created duplicate history")
	}
	if response := call(t, a, "DELETE", "/api/memories/"+saved.ID, nil); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	second := newTestSession(t, a)
	body = map[string]string{"sessionId": second.ID, "content": "新的话题", "requestId": "request-two"}
	if response := call(t, a, "POST", "/api/chat", body); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	mu.Lock()
	secondPrompt := received[1]
	mu.Unlock()
	encoded, _ := json.Marshal(secondPrompt)
	if strings.Contains(string(encoded), "第一段私有话题") || strings.Contains(string(encoded), "只喜欢安静的钢琴") {
		t.Fatal("old session or deleted memory leaked into new context")
	}
}

func TestCancelledReplyIsNotSaved(t *testing.T) {
	started := make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer provider.Close()
	a := testApp(t, Config{ChatBase: provider.URL, ChatKey: "secret", ChatModel: "test"})
	session := newTestSession(t, a)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	data, _ := json.Marshal(map[string]string{"sessionId": session.ID, "content": "这句话取消", "requestId": "cancel-me"})
	req := httptest.NewRequest("POST", "/api/chat", bytes.NewReader(data)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	done := make(chan struct{})
	go func() { a.routes().ServeHTTP(httptest.NewRecorder(), req); close(done) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("provider did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not stop request")
	}
	messages := a.store.snapshot().Sessions[0].Messages
	if len(messages) != 1 || messages[0].Role != "user" || messages[0].Status != "cancelled" {
		t.Fatalf("stale assistant reply persisted: %+v", messages)
	}
}

func TestProviderFailureDoesNotPretendToBeAI(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		_, _ = w.Write([]byte("private provider details and credentials"))
	}))
	defer provider.Close()
	a := testApp(t, Config{ChatBase: provider.URL, ChatKey: "private-key", ChatModel: "test"})
	session := newTestSession(t, a)
	response := call(t, a, "POST", "/api/chat", map[string]string{"sessionId": session.ID, "content": "你好", "requestId": "quota-request"})
	if response.Code != 502 || !strings.Contains(response.Body.String(), "额度") {
		t.Fatalf("failure not surfaced: %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "private") {
		t.Fatal("upstream details leaked")
	}
	messages := a.store.snapshot().Sessions[0].Messages
	if len(messages) != 1 || messages[0].Status != "failed" {
		t.Fatal("failure was replaced by a fabricated answer")
	}
	public := call(t, a, "GET", "/api/config", nil)
	if strings.Contains(public.Body.String(), "private-key") || strings.Contains(public.Body.String(), provider.URL) {
		t.Fatal("secret configuration exposed")
	}
}

func TestMemoryLimitAndIndependentHistoryDeletion(t *testing.T) {
	a := testApp(t, Config{})
	_ = newTestSession(t, a)
	if err := a.store.update(func(s *State) error {
		for range 100 {
			s.Memories = append(s.Memories, Memory{ID: newID(), Content: "保存的记忆"})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if response := call(t, a, "POST", "/api/memories", map[string]string{"content": "第101条"}); response.Code != 400 {
		t.Fatal("memory limit not enforced")
	}
	if response := call(t, a, "DELETE", "/api/sessions", nil); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	state := a.store.snapshot()
	if len(state.Sessions) != 0 || len(state.Memories) != 100 {
		t.Fatal("clearing history also deleted memories")
	}
}

func TestInputValidationAndCrossOriginRequests(t *testing.T) {
	a := testApp(t, Config{})
	if response := call(t, a, "PUT", "/api/settings", map[string]any{"nickname": "朋友", "volume": 101}); response.Code != 400 {
		t.Fatal("invalid volume accepted")
	}
	session := newTestSession(t, a)
	if response := call(t, a, "POST", "/api/chat", map[string]string{"sessionId": session.ID, "content": strings.Repeat("鲸", 1001), "requestId": "too-long"}); response.Code != 400 {
		t.Fatal("oversized message accepted")
	}
	req := httptest.NewRequest("POST", "http://localhost/api/sessions", nil)
	req.Header.Set("Origin", "https://another-site.example")
	recorder := httptest.NewRecorder()
	security(a.routes()).ServeHTTP(recorder, req)
	if recorder.Code != 403 {
		t.Fatal("cross-origin write accepted")
	}
}

func TestSpeechAdapterContracts(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer voice-secret" {
			t.Error("voice credential was not sent by backend")
		}
		switch r.URL.Path {
		case "/audio/transcriptions":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			defer r.MultipartForm.RemoveAll()
			if r.FormValue("model") != "asr-test" || r.FormValue("language") != "zh" {
				t.Error("incorrect ASR form fields")
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Error("missing audio upload")
				return
			}
			defer file.Close()
			data, _ := io.ReadAll(file)
			if header.Filename != "recording.webm" || string(data) != "test-audio-bytes" {
				t.Error("uploaded recording changed")
			}
			writeJSON(w, 200, map[string]string{"text": "今天想听一首歌"})
		case "/audio/speech":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["input"] != "我在听" || body["voice"] != "voice-test" || body["model"] != "tts-test" {
				t.Error("incorrect TTS payload")
			}
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("mock-audio-response"))
		default:
			t.Errorf("unexpected voice route: %s", r.URL.Path)
		}
	}))
	defer provider.Close()
	a := testApp(t, Config{ASRBase: provider.URL, ASRKey: "voice-secret", ASRModel: "asr-test", TTSBase: provider.URL, TTSKey: "voice-secret", TTSModel: "tts-test", TTSVoice: "voice-test"})
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	part, _ := writer.CreateFormFile("audio", "input.webm")
	_, _ = part.Write([]byte("test-audio-bytes"))
	_ = writer.Close()
	req := httptest.NewRequest("POST", "/api/transcribe", &buffer)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "今天想听一首歌") {
		t.Fatalf("ASR failed: %d %s", rec.Code, rec.Body.String())
	}
	response := call(t, a, "POST", "/api/speech", map[string]string{"text": "我在听"})
	if response.Code != 200 || response.Header().Get("Content-Type") != "audio/mpeg" || response.Body.String() != "mock-audio-response" {
		t.Fatal("TTS binary response was not forwarded")
	}
}
