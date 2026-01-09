package livekitclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jacky-htg/ai-call-center/libs/interfaces"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// RoomClient represents a LiveKit room client that can join as a participant
type RoomClient struct {
	url       string
	token     string
	roomName  string
	identity  string
	conn      *websocket.Conn
	pc        *webrtc.PeerConnection
	stt       interfaces.STT
	llm       interfaces.LLM
	tts       interfaces.TTS
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	audioTrack *webrtc.TrackLocalStaticSample
}

// NewRoomClient creates a new LiveKit room client
func NewRoomClient(url, token, roomName, identity string, stt interfaces.STT, llm interfaces.LLM, tts interfaces.TTS) *RoomClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &RoomClient{
		url:      url,
		token:    token,
		roomName: roomName,
		identity: identity,
		stt:      stt,
		llm:      llm,
		tts:      tts,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Connect joins the LiveKit room
func (rc *RoomClient) Connect() error {
	// Parse URL and convert to WebSocket URL
	wsURL := rc.url
	if len(wsURL) >= 5 && wsURL[:5] == "https" {
		wsURL = "wss" + wsURL[5:]
	} else if len(wsURL) >= 4 && wsURL[:4] == "http" {
		wsURL = "ws" + wsURL[4:]
	}
	
	// Append /rtc endpoint
	if wsURL[len(wsURL)-1] != '/' {
		wsURL += "/"
	}
	wsURL += "rtc?access_token=" + rc.token

	log.Printf("Connecting to LiveKit room: %s", wsURL)
	
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to dial websocket: %w", err)
	}
	rc.conn = conn

	// Start message handler
	go rc.handleMessages()

	// Create WebRTC peer connection
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}

	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return fmt.Errorf("failed to create peer connection: %w", err)
	}
	rc.pc = pc

	// Handle incoming audio tracks
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		if track.Kind() == webrtc.RTPCodecTypeAudio {
			log.Printf("Received audio track: %s", track.ID())
			go rc.handleAudioTrack(track)
		}
	})

	// Create audio track for publishing agent responses
	audioTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"agent-audio",
		"agent",
	)
	if err != nil {
		return fmt.Errorf("failed to create audio track: %w", err)
	}
	rc.audioTrack = audioTrack

	// Add track to peer connection
	if _, err := pc.AddTrack(audioTrack); err != nil {
		return fmt.Errorf("failed to add track: %w", err)
	}

	log.Printf("Successfully connected to room %s as %s", rc.roomName, rc.identity)
	return nil
}

// handleMessages processes WebSocket messages from LiveKit
func (rc *RoomClient) handleMessages() {
	defer rc.conn.Close()
	
	for {
		select {
		case <-rc.ctx.Done():
			return
		default:
			_, message, err := rc.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket error: %v", err)
				}
				return
			}

			var msg map[string]interface{}
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Printf("Failed to unmarshal message: %v", err)
				continue
			}

			// Handle different message types
			msgType, _ := msg["type"].(string)
			switch msgType {
			case "join":
				log.Printf("Joined room successfully")
			case "track_published":
				log.Printf("Track published: %v", msg)
			case "participant_connected":
				log.Printf("Participant connected: %v", msg)
			case "participant_disconnected":
				log.Printf("Participant disconnected: %v", msg)
			}
		}
	}
}

// handleAudioTrack processes incoming audio from user
func (rc *RoomClient) handleAudioTrack(track *webrtc.TrackRemote) {
	log.Printf("Starting to handle audio track: %s", track.ID())
	
	// Buffer for audio data
	audioBuffer := make([]byte, 0, 32000) // ~1 second at 16kHz
	bufferDuration := 2 * time.Second     // Process every 2 seconds
	ticker := time.NewTicker(bufferDuration)
	defer ticker.Stop()

	for {
		select {
		case <-rc.ctx.Done():
			return
		case <-ticker.C:
			if len(audioBuffer) > 0 {
				// Process audio chunk
				go rc.processAudioChunk(audioBuffer)
				audioBuffer = audioBuffer[:0] // Reset buffer
			}
		default:
			// Read RTP packet
			rtpPacket, _, err := track.ReadRTP()
			if err != nil {
				if err == io.EOF {
					log.Printf("Audio track ended")
					return
				}
				log.Printf("Error reading RTP: %v", err)
				continue
			}

			// Convert RTP to raw audio (simplified - in production, use proper codec decoder)
			// For MVP, we'll accumulate packets and process periodically
			audioBuffer = append(audioBuffer, rtpPacket.Payload...)
		}
	}
}

// processAudioChunk processes an audio chunk through STT -> LLM -> TTS pipeline
func (rc *RoomClient) processAudioChunk(audio []byte) {
	if len(audio) == 0 {
		return
	}

	// STT: Convert audio to text
	if rc.stt == nil {
		return
	}

	transcript, confidence, err := rc.stt.Recognize(audio)
	if err != nil {
		log.Printf("STT error: %v", err)
		return
	}

	if confidence < 0.5 || transcript == "" {
		return // Low confidence or empty transcript
	}

	log.Printf("User said: %s (confidence: %.2f)", transcript, confidence)

	// LLM: Generate response
	var response string
	if rc.llm != nil {
		response, err = rc.llm.Generate(transcript)
		if err != nil {
			log.Printf("LLM error: %v", err)
			response = "I'm sorry, I didn't catch that."
		}
	} else {
		response = "I heard you say: " + transcript
	}

	log.Printf("Agent response: %s", response)

	// TTS: Convert response to audio and publish
	if rc.tts != nil && rc.audioTrack != nil {
		audioData, err := rc.tts.Speak(response)
		if err != nil {
			log.Printf("TTS error: %v", err)
			return
		}

		// Publish audio to room (simplified - in production, use proper codec encoder)
		// For MVP, we'll send audio samples
		if err := rc.publishAudio(audioData); err != nil {
			log.Printf("Failed to publish audio: %v", err)
		}
	}
}

// publishAudio publishes audio data to the room
func (rc *RoomClient) publishAudio(audioData []byte) error {
	if rc.audioTrack == nil {
		return fmt.Errorf("audio track not initialized")
	}

	// Convert audio bytes to samples (simplified - assumes PCM format)
	// In production, you'd need proper audio format conversion
	// For MVP, we'll send raw samples
	sampleRate := uint32(16000) // 16kHz
	sampleDuration := time.Duration(len(audioData)) * time.Second / time.Duration(sampleRate*2) // Assuming 16-bit samples

	// Send audio samples
	chunkSize := int(sampleRate * 2 / 10) // 100ms chunks
	for i := 0; i < len(audioData); i += chunkSize {
		end := i + chunkSize
		if end > len(audioData) {
			end = len(audioData)
		}

		sample := media.Sample{
			Data:     audioData[i:end],
			Duration: sampleDuration / 10, // 100ms
		}

		if err := rc.audioTrack.WriteSample(sample); err != nil {
			return fmt.Errorf("failed to write sample: %w", err)
		}

		time.Sleep(100 * time.Millisecond) // Rate limit
	}

	return nil
}

// Disconnect leaves the room and cleans up
func (rc *RoomClient) Disconnect() error {
	rc.cancel()
	
	if rc.pc != nil {
		if err := rc.pc.Close(); err != nil {
			log.Printf("Error closing peer connection: %v", err)
		}
	}

	if rc.conn != nil {
		if err := rc.conn.Close(); err != nil {
			log.Printf("Error closing websocket: %v", err)
		}
	}

	return nil
}
