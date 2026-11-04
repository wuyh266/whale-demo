package main

import (
	"bufio"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

//go:embed web
var webFiles embed.FS

type Config struct {
	ChatBase, ChatKey, ChatModel string
	ASRBase, ASRKey, ASRModel    string
	TTSBase, TTSKey, TTSModel    string
	TTSVoice                     string
	TTSProvider                  string
	GPTSoVITSBase                string
	GPTSoVITSProfile             SoVITSProfile
	TTSTimeout                   time.Duration
}

func (c Config) liveChat() bool  { return c.ChatKey != "" && c.ChatBase != "" && c.ChatModel != "" }
func (c Config) serverASR() bool { return c.ASRKey != "" && c.ASRBase != "" && c.ASRModel != "" }
func (c Config) serverTTS() bool {
	if c.ttsProvider() == "gpt-sovits" {
		return c.GPTSoVITSBase != ""
	}
	if c.ttsProvider() != "openai" {
		return false
	}
	return c.TTSKey != "" && c.TTSBase != "" && c.TTSModel != "" && c.TTSVoice != ""
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func loadEnv(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.Trim(strings.TrimSpace(value), "\"'")
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	return scanner.Err()
}

func main() {
	if err := loadEnv(".env"); err != nil {
		log.Fatal("无法读取 .env 配置")
	}
	addr := flag.String("addr", env("APP_ADDR", "127.0.0.1:8787"), "local listening address")
	dataDir := flag.String("data", env("DATA_DIR", "data"), "local data directory")
	flag.Parse()
	cfg := Config{
		ChatBase: env("LLM_API_BASE", "https://api.deepseek.com"), ChatKey: env("LLM_API_KEY", ""), ChatModel: env("LLM_MODEL", "deepseek-flash"),
		ASRBase: env("ASR_API_BASE", ""), ASRKey: env("ASR_API_KEY", ""), ASRModel: env("ASR_MODEL", ""),
		TTSBase: env("TTS_API_BASE", ""), TTSKey: env("TTS_API_KEY", ""), TTSModel: env("TTS_MODEL", ""), TTSVoice: env("TTS_VOICE", ""),
	}
	if err := configureTTS(&cfg); err != nil {
		log.Fatal(err)
	}
	store, err := openStore(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	app := &App{store: store, config: cfg, client: &http.Client{Timeout: 45 * time.Second}}
	root, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatal(err)
	}
	mux := app.routes()
	mux.Handle("GET /", http.FileServer(http.FS(root)))
	writeTimeout := 100 * time.Second
	if cfg.speechTimeout()+15*time.Second > writeTimeout {
		writeTimeout = cfg.speechTimeout() + 15*time.Second
	}
	server := &http.Server{
		Addr: *addr, Handler: security(mux), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 20 * time.Second, WriteTimeout: writeTimeout, IdleTimeout: 90 * time.Second,
	}
	mode := "本地演示回复"
	if cfg.liveChat() {
		mode = "已连接 AI 对话服务"
	}
	fmt.Printf("\n鲸伴 Demo 已启动\n打开 http://%s\n%s\n按 Ctrl+C 停止服务\n\n", *addr, mode)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; media-src 'self' blob:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host && origin != "https://"+r.Host {
					writeError(w, 403, "请从鲸伴页面发起操作")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
