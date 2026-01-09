package agents

import (
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jacky-htg/ai-call-center/backend/internal/factory"
	"github.com/jacky-htg/ai-call-center/libs/config"
)

// This test performs a real end-to-end run against live vendor servers.
// It is intentionally disabled by default. To run it set the environment variable:
//
//	RUN_REAL_E2E=1
//
// and ensure OLLAMA_ENDPOINT, WHISPER_ENDPOINT, and PIPER endpoint (via VendorSettings or defaults)
// point to reachable services on your machine or network.
func TestHandleAudioFile_RealVendors(t *testing.T) {
	if os.Getenv("RUN_REAL_E2E") != "1" {
		t.Skip("real E2E tests disabled; set RUN_REAL_E2E=1 to enable")
	}

	cfg := config.LoadFromEnv()

	// Resolve endpoints to ensure services are reachable before running the long test.
	endpoints := []string{}
	// whisper
	if ws, ok := cfg.VendorSettings["whisper"]; ok {
		if ep := ws["endpoint"]; ep != "" {
			endpoints = append(endpoints, ep)
		}
	}
	// piper
	if ps, ok := cfg.VendorSettings["piper"]; ok {
		if ep := ps["endpoint"]; ep != "" {
			endpoints = append(endpoints, ep)
		}
	}
	// ollama
	if osMap, ok := cfg.VendorSettings["ollama"]; ok {
		if ep := osMap["endpoint"]; ep != "" {
			endpoints = append(endpoints, ep)
		}
	}

	// If no explicit endpoints found, use the defaults the factory will use.
	if len(endpoints) == 0 {
		endpoints = append(endpoints, "http://localhost:7070/inference") // whisper default
		endpoints = append(endpoints, "http://localhost:7071/tts")       // piper default
		endpoints = append(endpoints, "http://localhost:11434/api/generate")
	}

	// Quick reachability (TCP) check for each endpoint host:port.
	for _, ep := range endpoints {
		u, err := url.Parse(ep)
		if err != nil {
			t.Fatalf("invalid endpoint URL %q: %v", ep, err)
		}
		host := u.Host
		if host == "" {
			t.Fatalf("empty host for endpoint %q", ep)
		}
		if _, _, err := net.SplitHostPort(host); err != nil {
			// add default port based on scheme
			if u.Scheme == "https" {
				host = host + ":443"
			} else {
				host = host + ":80"
			}
		}
		d := 3 * time.Second
		conn, err := net.DialTimeout("tcp", host, d)
		if err != nil {
			t.Fatalf("endpoint %s not reachable (tcp %s): %v", ep, host, err)
		}
		conn.Close()
	}

	// Construct real clients via factory
	tts, err := factory.NewTTS(cfg)
	if err != nil {
		t.Fatalf("new tts: %v", err)
	}
	stt, err := factory.NewSTT(cfg)
	if err != nil {
		t.Fatalf("new stt: %v", err)
	}
	llm, err := factory.NewLLM(cfg)
	if err != nil {
		t.Fatalf("new llm: %v", err)
	}
	webrtc, err := factory.NewWebRTC(cfg)
	if err != nil {
		t.Fatalf("new webrtc: %v", err)
	}

	ag := New(tts, stt, llm, webrtc)

	in := filepath.Join("testdata", "jfk.wav")
	if _, err := os.Stat(in); err != nil {
		t.Fatalf("input audio not found: %v", err)
	}

	out := filepath.Join(os.TempDir(), "ai-call-e2e-output.wav")
	// remove any previous output
	_ = os.Remove(out)

	if err := ag.HandleAudioFile(in, out); err != nil {
		t.Fatalf("HandleAudioFile failed: %v", err)
	}

	fi, err := os.Stat(out)
	if err != nil {
		t.Fatalf("output file missing: %v", err)
	}
	if fi.Size() == 0 {
		t.Fatalf("output file is empty")
	}
}
