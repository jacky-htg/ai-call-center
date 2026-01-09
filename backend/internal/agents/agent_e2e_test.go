package agents

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jacky-htg/ai-call-center/libs/vendors/livekit"
	"github.com/jacky-htg/ai-call-center/libs/vendors/ollama"
	"github.com/jacky-htg/ai-call-center/libs/vendors/piper"
	"github.com/jacky-htg/ai-call-center/libs/vendors/whisper"
)

// This integration test spins up lightweight HTTP servers that mimic the vendor
// endpoints (Whisper STT, Ollama LLM, Piper TTS) and runs CallAgent.HandleAudioFile
// to verify end-to-end flow writes a non-empty audio file.
func TestHandleAudioFile_E2E_SimulatedVendors(t *testing.T) {
	// Whisper fake: accept multipart POST and return JSON {"text": "simulated transcript"}
	whisperSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = r.ParseMultipartForm(10 << 20)
		// ignore actual file contents for test
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "And so my fellow Americans ..."})
	}))
	defer whisperSrv.Close()

	// Ollama fake: accept JSON {model,prompt,stream} and return {response: ...}
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		prompt := ""
		if p, ok := req["prompt"].(string); ok {
			prompt = p
		}
		resp := map[string]interface{}{"response": "LLM answer to: " + prompt}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ollamaSrv.Close()

	// Piper fake: accept form value 'text' and stream some bytes as audio
	piperSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		text := r.FormValue("text")
		w.Header().Set("Content-Type", "audio/wav")
		// simple non-empty payload; in real tests you could write a valid WAV header
		_, _ = io.WriteString(w, "WAVDATA:"+text)
	}))
	defer piperSrv.Close()

	// Build vendor clients pointing at our test servers
	tts := piper.NewWithEndpoint(piperSrv.URL)
	stt := whisper.NewWithEndpoint(whisperSrv.URL)
	llm := ollama.NewWithEndpointModel(ollamaSrv.URL, "tinyllama")
	webrtc := livekit.New()

	ag := New(tts, stt, llm, webrtc)

	// Input audio file (testdata/jfk.wav should exist in repository)
	in := filepath.Join("testdata", "jfk.wav")
	if _, err := os.Stat(in); err != nil {
		t.Skipf("input test audio not present: %v", err)
	}

	out, err := os.CreateTemp("", "ai-call-out-*.wav")
	if err != nil {
		t.Fatalf("create temp out: %v", err)
	}
	outPath := out.Name()
	out.Close()
	defer os.Remove(outPath)

	if err := ag.HandleAudioFile(in, outPath); err != nil {
		t.Fatalf("HandleAudioFile failed: %v", err)
	}

	fi, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("output file missing: %v", err)
	}
	if fi.Size() == 0 {
		t.Fatalf("output file is empty")
	}

	// quick content check: ensure the output contains the LLM/TTS text marker
	b, _ := os.ReadFile(outPath)
	if !strings.Contains(string(b), "WAVDATA:") {
		t.Fatalf("unexpected output content: %q", string(b))
	}
}
