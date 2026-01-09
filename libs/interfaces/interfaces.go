package interfaces

import "io"

// TTS is the text-to-speech interface. Implementations should be swappable.
type TTS interface {
	// Speak converts text into audio bytes (e.g., encoded PCM or an audio format)
	Speak(text string, opts ...TTSOption) ([]byte, error)
	// SpeakStream writes audio bytes for the given text to the provided writer as they are produced.
	// Implementations that can stream should provide this for low-latency playback.
	SpeakStream(text string, w io.Writer, opts ...TTSOption) error
}

// STT is the speech-to-text interface.
type STT interface {
	// Recognize converts audio bytes into text (returns transcript and confidence)
	Recognize(audio []byte, opts ...STTOption) (string, float32, error)
}

// LLM is the language model interface.
type LLM interface {
	// Generate takes a prompt and returns a generated text response
	Generate(prompt string, opts ...LLMOption) (string, error)
}

// WebRTCProvider represents actions needed to manage a WebRTC session (signaling/rooms)
type WebRTCProvider interface {
	// StartSession creates/initializes a session and returns a session ID or error
	StartSession(opts ...WebRTCOption) (string, error)
	// StopSession cleanly closes the session
	StopSession(sessionID string) error
}

// Option types are intentionally small placeholders to allow vendor-specific options.
type TTSOption func(*map[string]any)
type STTOption func(*map[string]any)
type LLMOption func(*map[string]any)
type WebRTCOption func(*map[string]any)
