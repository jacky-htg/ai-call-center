package livekit

import "github.com/jacky-htg/ai-call-center/libs/interfaces"

type livekitProvider struct{}

func New() interfaces.WebRTCProvider { return &livekitProvider{} }

func (l *livekitProvider) StartSession(opts ...interfaces.WebRTCOption) (string, error) {
	// Stub: return a fake session id
	return "livekit-session-stub", nil
}

func (l *livekitProvider) StopSession(sessionID string) error {
	// No-op for stub
	return nil
}
