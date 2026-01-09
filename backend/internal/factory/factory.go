package factory

import (
	"errors"

	"github.com/jacky-htg/ai-call-center/libs/config"
	"github.com/jacky-htg/ai-call-center/libs/interfaces"
	"github.com/jacky-htg/ai-call-center/libs/vendors/livekit"
	"github.com/jacky-htg/ai-call-center/libs/vendors/ollama"
	"github.com/jacky-htg/ai-call-center/libs/vendors/piper"
	"github.com/jacky-htg/ai-call-center/libs/vendors/whisper"
)

func NewTTS(cfg *config.Config) (interfaces.TTS, error) {
	switch cfg.TTSVendor {
	case "piper":
		// Allow endpoint override via VendorSettings["piper"]["endpoint"]
		if cfg != nil && cfg.VendorSettings != nil {
			if ps, ok := cfg.VendorSettings["piper"]; ok {
				if ep, ok := ps["endpoint"]; ok && ep != "" {
					return piper.NewWithEndpoint(ep), nil
				}
			}
		}
		return piper.New(), nil
	default:
		return nil, errors.New("unknown tts vendor")
	}
}

func NewSTT(cfg *config.Config) (interfaces.STT, error) {
	switch cfg.STTVendor {
	case "whisper":
		// Allow endpoint override via VendorSettings["whisper"]["endpoint"]
		if cfg != nil && cfg.VendorSettings != nil {
			if ws, ok := cfg.VendorSettings["whisper"]; ok {
				if ep, ok := ws["endpoint"]; ok && ep != "" {
					return whisper.NewWithEndpoint(ep), nil
				}
			}
		}
		return whisper.New(), nil
	default:
		return nil, errors.New("unknown stt vendor")
	}
}

func NewLLM(cfg *config.Config) (interfaces.LLM, error) {
	switch cfg.LLMVendor {
	case "ollama":
		// Allow endpoint/model override via VendorSettings["ollama"]
		if cfg != nil && cfg.VendorSettings != nil {
			if os, ok := cfg.VendorSettings["ollama"]; ok {
				ep := os["endpoint"]
				model := os["model"]
				if ep != "" || model != "" {
					return ollama.NewWithEndpointModel(ep, model), nil
				}
			}
		}
		return ollama.New(), nil
	default:
		return nil, errors.New("unknown llm vendor")
	}
}

func NewWebRTC(cfg *config.Config) (interfaces.WebRTCProvider, error) {
	switch cfg.WebRTCVendor {
	case "livekit":
		return livekit.New(), nil
	default:
		return nil, errors.New("unknown webrtc vendor")
	}
}
