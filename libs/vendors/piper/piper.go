package piper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jacky-htg/ai-call-center/libs/interfaces"
)

// piperTTS is the primary Piper implementation in this package: Piper as TTS.
type piperTTS struct {
	endpoint string
	client   *http.Client
}

// New returns a Piper TTS implementation with the default local endpoint.
func New() interfaces.TTS { return NewWithEndpoint("http://localhost:7071/tts") }

// NewWithEndpoint allows overriding the Piper TTS endpoint.
func NewWithEndpoint(endpoint string) interfaces.TTS {
	if endpoint == "" {
		endpoint = "http://localhost:7071/tts"
	}
	// Use a larger timeout because the Piper binary may take time to start and stream audio.
	return &piperTTS{endpoint: endpoint, client: &http.Client{Timeout: 120 * time.Second}}
}

type ttsRequest struct {
	Text string `json:"text"`
}

func (p *piperTTS) Speak(text string, opts ...interfaces.TTSOption) ([]byte, error) {
	// Primary: send url-encoded form with field "text" to match server's r.FormValue("text")
	form := url.Values{}
	form.Set("text", text)
	resp, err := p.client.Post(p.endpoint, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("post form to piper tts: %w", err)
	}
	defer resp.Body.Close()

	// Read streaming response (server writes chunked WAV). We read fully into memory for
	// simplicity; for large streams you may want to stream directly to disk.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read tts response: %w", err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return body, nil
	}

	// Fallbacks if form didn't work: try JSON then text/plain then GET
	reqBody, _ := json.Marshal(ttsRequest{Text: text})
	resp2, err := p.client.Post(p.endpoint, "application/json", bytes.NewReader(reqBody))
	if err == nil {
		defer resp2.Body.Close()
		if resp2.StatusCode >= 200 && resp2.StatusCode < 300 {
			b2, _ := io.ReadAll(resp2.Body)
			return b2, nil
		}
	}

	resp3, err := p.client.Post(p.endpoint, "text/plain", strings.NewReader(text))
	if err == nil {
		defer resp3.Body.Close()
		if resp3.StatusCode >= 200 && resp3.StatusCode < 300 {
			b3, _ := io.ReadAll(resp3.Body)
			return b3, nil
		}
	}

	getURL := p.endpoint
	if strings.Contains(getURL, "?") {
		getURL = getURL + "&text=" + url.QueryEscape(text)
	} else {
		getURL = getURL + "?text=" + url.QueryEscape(text)
	}
	resp4, err := p.client.Get(getURL)
	if err == nil {
		defer resp4.Body.Close()
		if resp4.StatusCode >= 200 && resp4.StatusCode < 300 {
			b4, _ := io.ReadAll(resp4.Body)
			return b4, nil
		}
	}

	return nil, fmt.Errorf("piper tts request failed, last status %d", resp.StatusCode)
}

// SpeakStream streams audio produced by the Piper server directly to the provided writer.
// This avoids buffering large audio in memory and enables low-latency playback.
func (p *piperTTS) SpeakStream(text string, w io.Writer, opts ...interfaces.TTSOption) error {
	form := url.Values{}
	form.Set("text", text)
	resp, err := p.client.Post(p.endpoint, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("post form to piper tts: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("piper tts bad status %d: %s", resp.StatusCode, string(b))
	}

	// Copy the streaming response body to the writer until EOF.
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("stream tts response: %w", err)
	}
	return nil
}

// Keep a legacy STT stub available as NewSTT if someone needs it.
type piperSTT struct{}

func NewSTT() interfaces.STT { return &piperSTT{} }

func (p *piperSTT) Recognize(audio []byte, opts ...interfaces.STTOption) (string, float32, error) {
	return "transcript from piper (stub)", 0.93, nil
}
