package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxCompanionImage = 5 << 20

var companionImageName = regexp.MustCompile(`^[a-f0-9]{24}\.(png|jpg|webp)$`)

func (a *App) imageDirectory() string {
	return filepath.Join(filepath.Dir(a.store.path), "uploads")
}

func (a *App) prepareCompanionVisuals(input, previous *Companion, theme, imageURL, voice *string) error {
	input.Theme, input.ImageURL, input.Voice = "ocean", "", "default"
	if previous != nil {
		input.Theme, input.ImageURL, input.Voice = previous.Theme, previous.ImageURL, previous.Voice
	}
	if theme != nil {
		input.Theme = strings.TrimSpace(*theme)
	}
	if imageURL != nil {
		input.ImageURL = strings.TrimSpace(*imageURL)
	}
	if voice != nil {
		input.Voice = strings.TrimSpace(*voice)
	}
	if input.Theme == "" {
		input.Theme = "ocean"
	}
	if input.Voice == "" {
		input.Voice = "default"
	}
	switch input.Theme {
	case "anon", "ocean", "forest", "violet":
	default:
		return errors.New("请选择爱音粉、海蓝、森林绿或紫罗兰主题")
	}
	switch input.Voice {
	case "default", "anon", "soyo", "sakiko_white", "browser":
	default:
		return errors.New("请选择默认音色、千早爱音、长崎素世、客服小祥或设备声音")
	}
	if input.ImageURL == "" {
		input.ImageURL = "/assets/whale-girl-sheet.png"
		if input.Theme == "anon" {
			input.ImageURL = "/assets/anon.png"
		}
	}
	if input.ImageURL == "/assets/anon.png" || input.ImageURL == "/assets/whale-girl-sheet.png" {
		return nil
	}
	name, ok := strings.CutPrefix(input.ImageURL, "/api/companion-images/")
	if !ok || !companionImageName.MatchString(name) {
		return errors.New("请使用内置图片或在此页面上传伴侣参考图")
	}
	info, err := os.Stat(filepath.Join(a.imageDirectory(), name))
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("参考图不存在，请重新上传后保存")
	}
	return nil
}

func (a *App) uploadCompanionImage(w http.ResponseWriter, r *http.Request) {
	// The small allowance is multipart framing; the image itself is checked below.
	r.Body = http.MaxBytesReader(w, r.Body, maxCompanionImage+(64<<10))
	if err := r.ParseMultipartForm(maxCompanionImage + (64 << 10)); err != nil {
		writeError(w, 400, "图片过大或无法读取，请选择不超过5MB的PNG、JPEG或WebP图片")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, _, err := r.FormFile("image")
	if err != nil {
		writeError(w, 400, "请选择一张伴侣参考图")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxCompanionImage+1))
	if err != nil || len(data) == 0 || len(data) > maxCompanionImage {
		writeError(w, 400, "参考图不能为空，且不能超过5MB")
		return
	}
	format, width, height, err := companionImageDimensions(data)
	if err != nil || width < 1 || height < 1 || width > 4096 || height > 4096 || int64(width)*int64(height) > 16_000_000 {
		writeError(w, 400, "请上传有效的PNG、JPEG或静态WebP图片，长宽不超过4096像素且总像素不超过1600万")
		return
	}
	if err := os.MkdirAll(a.imageDirectory(), 0700); err != nil {
		writeError(w, 500, "参考图暂时无法保存，请检查本地空间")
		return
	}
	name := newID() + "." + format
	path := filepath.Join(a.imageDirectory(), name)
	// The name is generated here; never use a filename supplied by the browser.
	if err := os.WriteFile(path, data, 0600); err != nil {
		writeError(w, 500, "参考图暂时没有保存成功")
		return
	}
	writeJSON(w, 201, map[string]string{"imageUrl": "/api/companion-images/" + name})
}

func (a *App) companionImage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !companionImageName.MatchString(name) {
		writeError(w, 404, "没有找到这张伴侣参考图")
		return
	}
	file, err := os.Open(filepath.Join(a.imageDirectory(), name))
	if err != nil {
		writeError(w, 404, "没有找到这张伴侣参考图")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		writeError(w, 404, "没有找到这张伴侣参考图")
		return
	}
	switch filepath.Ext(name) {
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".webp":
		w.Header().Set("Content-Type", "image/webp")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, name, info.ModTime(), file)
}

func companionImageDimensions(data []byte) (format string, width, height int, err error) {
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		width, height, err = webPDimensions(data)
		return "webp", width, height, err
	}
	config, name, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (name != "png" && name != "jpeg") {
		return "", 0, 0, errors.New("unsupported or invalid image")
	}
	// Decode only after enforcing header dimensions to avoid decompression bombs.
	if config.Width < 1 || config.Height < 1 || config.Width > 4096 || config.Height > 4096 || int64(config.Width)*int64(config.Height) > 16_000_000 {
		return "", 0, 0, errors.New("image dimensions exceeded")
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return "", 0, 0, err
	}
	if name == "jpeg" {
		name = "jpg"
	}
	return name, config.Width, config.Height, nil
}

// WebP is RIFF-based. Read its bounded frame header without adding an external
// codec dependency; browsers perform the final pixel decoding. Animated WebP is
// excluded so a single image cannot introduce unbounded animation workloads.
func webPDimensions(data []byte) (int, int, error) {
	invalid := errors.New("invalid or animated WebP")
	if len(data) < 20 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" || uint64(binary.LittleEndian.Uint32(data[4:8]))+8 != uint64(len(data)) {
		return 0, 0, invalid
	}
	width, height, canvasWidth, canvasHeight := 0, 0, 0, 0
	for offset := 12; offset < len(data); {
		if len(data)-offset < 8 {
			return 0, 0, invalid
		}
		size := uint64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		end := uint64(offset) + 8 + size
		if end > uint64(len(data)) {
			return 0, 0, invalid
		}
		chunk := data[offset+8 : int(end)]
		switch string(data[offset : offset+4]) {
		case "ANIM", "ANMF":
			return 0, 0, invalid
		case "VP8X":
			if len(chunk) != 10 || chunk[0]&2 != 0 {
				return 0, 0, invalid
			}
			canvasWidth = 1 + int(chunk[4]) + int(chunk[5])<<8 + int(chunk[6])<<16
			canvasHeight = 1 + int(chunk[7]) + int(chunk[8])<<8 + int(chunk[9])<<16
		case "VP8 ":
			if width != 0 || len(chunk) < 10 || chunk[0]&1 != 0 || !bytes.Equal(chunk[3:6], []byte{0x9d, 0x01, 0x2a}) {
				return 0, 0, invalid
			}
			width = int(binary.LittleEndian.Uint16(chunk[6:8]) & 0x3fff)
			height = int(binary.LittleEndian.Uint16(chunk[8:10]) & 0x3fff)
		case "VP8L":
			if width != 0 || len(chunk) < 5 || chunk[0] != 0x2f {
				return 0, 0, invalid
			}
			bits := binary.LittleEndian.Uint32(chunk[1:5])
			if bits>>29 != 0 {
				return 0, 0, invalid
			}
			width, height = 1+int(bits&0x3fff), 1+int((bits>>14)&0x3fff)
		}
		offset = int(end + size%2)
		if offset > len(data) {
			return 0, 0, invalid
		}
	}
	if width < 1 || height < 1 || (canvasWidth != 0 && (canvasWidth != width || canvasHeight != height)) {
		return 0, 0, invalid
	}
	return width, height, nil
}
