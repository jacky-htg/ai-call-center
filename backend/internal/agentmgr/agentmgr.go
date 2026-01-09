package agentmgr

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/jacky-htg/ai-call-center/backend/internal/livekitclient"
	"github.com/jacky-htg/ai-call-center/libs/config"
	"github.com/jacky-htg/ai-call-center/libs/interfaces"
	"github.com/jacky-htg/ai-call-center/libs/livekit"
	"github.com/jacky-htg/ai-call-center/libs/store"
)

// AgentManager manages AI agent sessions for calls. It's a light-weight in-memory
// manager that creates agent sessions in the store and tracks their lifecycle.
type AgentManager struct {
	mu sync.Mutex
	// map callID -> agentSessionID
	agents  map[string]string
	// map callID -> roomClient
	clients  map[string]*livekitclient.RoomClient
	cancels map[string]context.CancelFunc
	store   *store.Store
	cfg     *config.Config
	tts     interfaces.TTS
	llm     interfaces.LLM
	stt     interfaces.STT
}

// New creates an AgentManager. tts and llm are used by the background agent worker to produce audio.
func New(s *store.Store, cfg *config.Config, tts interfaces.TTS, llm interfaces.LLM, stt interfaces.STT) *AgentManager {
	return &AgentManager{
		agents:  make(map[string]string),
		clients: make(map[string]*livekitclient.RoomClient),
		cancels: make(map[string]context.CancelFunc),
		store:   s,
		cfg:     cfg,
		tts:     tts,
		llm:     llm,
		stt:     stt,
	}
}

// SpawnAgent creates an agent session for the call, marks it active and connects to LiveKit room.
// It returns the sessionID and a LiveKit token.
func (m *AgentManager) SpawnAgent(callID string) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.agents[callID]; ok {
		return "", "", fmt.Errorf("agent already exists for call %s", callID)
	}

	agentUser := "ai-agent"
	sessionID, err := m.store.CreateSession(callID, agentUser, "agent", "new")
	if err != nil {
		return "", "", err
	}

	// generate token for agent to join; use livekit settings from cfg
	lk := m.cfg.VendorSettings["livekit"]
	apiKey, apiSecret, url := "", "", ""
	if lk != nil {
		apiKey = lk["api_key"]
		apiSecret = lk["api_secret"]
		url = lk["url"]
	}
	if url == "" {
		return "", "", fmt.Errorf("livekit url not configured")
	}
	
	token, err := livekit.GenerateAccessToken(apiKey, apiSecret, callID, sessionID, 3600)
	if err != nil {
		return "", "", err
	}

	// persist the agent token so external agent workers can retrieve it
	_ = m.store.UpdateSessionToken(sessionID, token)

	// mark active
	_ = m.store.UpdateSessionStatus(sessionID, "active")

	// Create and connect room client
	ctx, cancel := context.WithCancel(context.Background())
	roomClient := livekitclient.NewRoomClient(url, token, callID, sessionID, m.stt, m.llm, m.tts)
	
	m.agents[callID] = sessionID
	m.clients[callID] = roomClient
	m.cancels[callID] = cancel

	// Connect to room in background
	go func() {
		if err := roomClient.Connect(); err != nil {
			log.Printf("Failed to connect agent to room %s: %v", callID, err)
			m.mu.Lock()
			delete(m.agents, callID)
			delete(m.clients, callID)
			delete(m.cancels, callID)
			m.mu.Unlock()
			_ = m.store.UpdateSessionStatus(sessionID, "ended")
			return
		}

		// Wait for context cancellation
		<-ctx.Done()
		
		// Disconnect and cleanup
		if err := roomClient.Disconnect(); err != nil {
			log.Printf("Error disconnecting agent from room %s: %v", callID, err)
		}
		_ = m.store.UpdateSessionStatus(sessionID, "ended")
	}()

	return sessionID, token, nil
}

// StopAgent stops the agent for the given call and marks it ended.
func (m *AgentManager) StopAgent(callID string) error {
	m.mu.Lock()
	cancel, ok := m.cancels[callID]
	sessionID := m.agents[callID]
	client := m.clients[callID]
	delete(m.cancels, callID)
	delete(m.agents, callID)
	delete(m.clients, callID)
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("no agent for call %s", callID)
	}
	
	// Cancel context to stop goroutine
	cancel()
	
	// Disconnect room client if exists
	if client != nil {
		if err := client.Disconnect(); err != nil {
			log.Printf("Error disconnecting client: %v", err)
		}
	}
	
	// session status will be updated by goroutine; but ensure it's ended
	_ = m.store.UpdateSessionStatus(sessionID, "ended")
	return nil
}

// ProcessIncomingAudio accepts raw audio bytes (from an external agent worker or media pipeline)
// and runs STT -> LLM -> TTS. It returns the transcript produced by STT.
func (m *AgentManager) ProcessIncomingAudio(sessionID string, audio []byte) (string, error) {
	if m.stt == nil {
		return "", fmt.Errorf("stt not configured")
	}

	// run STT
	transcript, _, err := m.stt.Recognize(audio)
	if err != nil {
		return "", err
	}

	// optionally generate LLM response
	var reply string
	if m.llm != nil {
		r, err := m.llm.Generate(transcript)
		if err == nil {
			reply = r
		}
	}
	if reply == "" {
		reply = "I heard you. Let me know if you'd like help."
	}

	// synthesize reply
	if m.tts != nil {
		audioOut, err := m.tts.Speak(reply)
		if err == nil && len(audioOut) > 0 {
			outDir := filepath.Join("out", "agents")
			_ = os.MkdirAll(outDir, 0755)
			fname := filepath.Join(outDir, fmt.Sprintf("agent-reply-%s-%d.wav", sessionID, time.Now().Unix()))
			_ = os.WriteFile(fname, audioOut, 0644)
		}
	}

	return transcript, nil
}
