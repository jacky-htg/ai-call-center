package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Config contains runtime configuration and vendor selection.
type Config struct {
	// Vendor keys: e.g., "whisper", "deepgram", "google_tts"
	TTSVendor    string `json:"tts_vendor"`
	STTVendor    string `json:"stt_vendor"`
	LLMVendor    string `json:"llm_vendor"`
	WebRTCVendor string `json:"webrtc_vendor"`

	// Generic map for vendor-specific settings
	VendorSettings map[string]map[string]string `json:"vendor_settings"`
}

// LoadFromEnv constructs a Config reading from environment variables.
// Supported env vars:
//
//	TTS_VENDOR, STT_VENDOR, LLM_VENDOR, WEBRTC_VENDOR
//	WHISPER_ENDPOINT - optional override for whisper STT endpoint (e.g. http://localhost:7070/inference)
//
// Additional vendor-specific variables may be added in the future.
func LoadFromEnv() *Config {
	cfg := &Config{
		TTSVendor:      getEnv("TTS_VENDOR", "piper"),
		STTVendor:      getEnv("STT_VENDOR", "whisper"),
		LLMVendor:      getEnv("LLM_VENDOR", "ollama"),
		WebRTCVendor:   getEnv("WEBRTC_VENDOR", "livekit"),
		VendorSettings: make(map[string]map[string]string),
	}

	// Whisper endpoint override
	if ep := getEnv("WHISPER_ENDPOINT", ""); ep != "" {
		cfg.VendorSettings["whisper"] = map[string]string{"endpoint": ep}
	}

	// Ollama optional overrides
	if ep := getEnv("OLLAMA_ENDPOINT", ""); ep != "" {
		if cfg.VendorSettings == nil {
			cfg.VendorSettings = make(map[string]map[string]string)
		}
		if _, ok := cfg.VendorSettings["ollama"]; !ok {
			cfg.VendorSettings["ollama"] = make(map[string]string)
		}
		cfg.VendorSettings["ollama"]["endpoint"] = ep
	}
	if model := getEnv("OLLAMA_MODEL", ""); model != "" {
		if cfg.VendorSettings == nil {
			cfg.VendorSettings = make(map[string]map[string]string)
		}
		if _, ok := cfg.VendorSettings["ollama"]; !ok {
			cfg.VendorSettings["ollama"] = make(map[string]string)
		}
		cfg.VendorSettings["ollama"]["model"] = model
	}

	// LiveKit settings
	if ep := getEnv("LIVEKIT_URL", ""); ep != "" {
		if cfg.VendorSettings == nil {
			cfg.VendorSettings = make(map[string]map[string]string)
		}
		if _, ok := cfg.VendorSettings["livekit"]; !ok {
			cfg.VendorSettings["livekit"] = make(map[string]string)
		}
		cfg.VendorSettings["livekit"]["url"] = ep
	}
	if k := getEnv("LIVEKIT_API_KEY", ""); k != "" {
		if cfg.VendorSettings == nil {
			cfg.VendorSettings = make(map[string]map[string]string)
		}
		if _, ok := cfg.VendorSettings["livekit"]; !ok {
			cfg.VendorSettings["livekit"] = make(map[string]string)
		}
		cfg.VendorSettings["livekit"]["api_key"] = k
	}
	if s := getEnv("LIVEKIT_API_SECRET", ""); s != "" {
		if cfg.VendorSettings == nil {
			cfg.VendorSettings = make(map[string]map[string]string)
		}
		if _, ok := cfg.VendorSettings["livekit"]; !ok {
			cfg.VendorSettings["livekit"] = make(map[string]string)
		}
		cfg.VendorSettings["livekit"]["api_secret"] = s
	}

	return cfg
}

func getEnv(key, def string) string {
	v := ""
	if val, ok := lookupEnv(key); ok {
		v = val
	} else {
		// fallback to .env file if present
		loadDotEnvOnce.Do(loadDotEnv)
		if dotEnv != nil {
			if val2, ok := dotEnv[key]; ok && val2 != "" {
				v = val2
			}
		}
	}
	if v == "" {
		return def
	}
	return v
}

// lookupEnv is a thin wrapper over os.LookupEnv so tests can replace it if needed.
var lookupEnv = func(key string) (string, bool) { return os.LookupEnv(key) }

var (
	dotEnv         map[string]string
	loadDotEnvOnce sync.Once
)

// loadDotEnv loads a .env file from the repository root (current working dir)
// and populates the dotEnv map. It ignores lines starting with '#' and empty lines.
func loadDotEnv() {
	// look for .env in current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	path := filepath.Join(cwd, ".env")
	data, err := os.ReadFile(path)
	if err != nil {
		// no .env present - nothing to do
		return
	}

	m := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// split at first '='
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:idx])
		v := strings.TrimSpace(line[idx+1:])
		// remove surrounding quotes if present
		if len(v) >= 2 {
			if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
				v = v[1 : len(v)-1]
			}
		}
		m[k] = v
	}
	dotEnv = m
}
