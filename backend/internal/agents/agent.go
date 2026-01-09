package agents

import (
	"fmt"
	"os"

	"github.com/jacky-htg/ai-call-center/libs/interfaces"
)

// CallAgent coordinates STT, LLM, and TTS for a single call/session.
type CallAgent struct {
	tts    interfaces.TTS
	stt    interfaces.STT
	llm    interfaces.LLM
	webrtc interfaces.WebRTCProvider
}

// New constructs a CallAgent with concrete components (injected via factory).
func New(tts interfaces.TTS, stt interfaces.STT, llm interfaces.LLM, webrtc interfaces.WebRTCProvider) *CallAgent {
	return &CallAgent{tts: tts, stt: stt, llm: llm, webrtc: webrtc}
}

// HandleAudioFile runs a simple end-to-end flow using a local audio file:
// 1) read audio bytes
// 2) STT -> transcript
// 3) LLM -> response
// 4) TTS -> audio bytes (written to output)
func (c *CallAgent) HandleAudioFile(inputPath, outputPath string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read input audio: %w", err)
	}

	transcript, conf, err := c.stt.Recognize(data)
	if err != nil {
		return fmt.Errorf("stt recognize: %w", err)
	}
	fmt.Printf("STT transcript (conf=%.2f): %s\n", conf, transcript)

	resp, err := c.llm.Generate(transcript)
	if err != nil {
		return fmt.Errorf("llm generate: %w", err)
	}
	fmt.Printf("LLM response: %s\n", resp)

	// Prefer streaming TTS to avoid buffering large audio in memory.
	outF, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer outF.Close()

	// Try to stream; if the TTS implementation fails to stream, fall back to Speak.
	if err := c.tts.SpeakStream(resp, outF); err != nil {
		// Fallback: attempt to get full bytes and write them
		outAudio, err2 := c.tts.Speak(resp)
		if err2 != nil {
			return fmt.Errorf("tts speak (stream failed: %v, fallback failed: %v)", err, err2)
		}
		if _, err3 := outF.Write(outAudio); err3 != nil {
			return fmt.Errorf("write output audio (fallback): %w", err3)
		}
	}

	fmt.Printf("Wrote output audio to %s\n", outputPath)
	return nil
}
