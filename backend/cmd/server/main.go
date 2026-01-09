package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jacky-htg/ai-call-center/backend/internal/agentmgr"
	"github.com/jacky-htg/ai-call-center/backend/internal/agents"
	"github.com/jacky-htg/ai-call-center/backend/internal/factory"
	"github.com/jacky-htg/ai-call-center/libs/config"
	livekitauth "github.com/jacky-htg/ai-call-center/libs/livekit"
	"github.com/jacky-htg/ai-call-center/libs/store"

	_ "modernc.org/sqlite"
)

func main() {
	fmt.Println("ai-call-center server (demo) starting")

	// Load config from environment (with sane defaults). You can override using env vars:
	// TTS_VENDOR, STT_VENDOR, LLM_VENDOR, WEBRTC_VENDOR, WHISPER_ENDPOINT
	cfg := config.LoadFromEnv()

	tts, err := factory.NewTTS(cfg)
	if err != nil {
		log.Fatalf("new tts: %v", err)
	}
	stt, err := factory.NewSTT(cfg)
	if err != nil {
		log.Fatalf("new stt: %v", err)
	}
	llm, err := factory.NewLLM(cfg)
	if err != nil {
		log.Fatalf("new llm: %v", err)
	}
	webrtc, err := factory.NewWebRTC(cfg)
	if err != nil {
		log.Fatalf("new webrtc: %v", err)
	}

	agent := agents.New(tts, stt, llm, webrtc)

	// Open SQLite DB
	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "data/ai.callcenter.db"
	}
	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}
	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer st.Close()

	// agent manager handles creating/stopping AI agent sessions (logical join/leave)
	mgr := agentmgr.New(st, cfg, tts, llm, stt)

	// Ensure output dir exists
	outDir := "out"
	if err := os.MkdirAll(outDir, 0755); err != nil {
		log.Fatalf("mkdir out: %v", err)
	}

	// Prefer a real WAV test file if present
	inputPath := filepath.Join("testdata", "sample.raw")
	jfkPath := filepath.Join("testdata", "jfk.wav")
	if _, err := os.Stat(jfkPath); err == nil {
		inputPath = jfkPath
		fmt.Println("Using testdata/jfk.wav as input")
	}
	outputPath := filepath.Join(outDir, "output.wav")

	if err := agent.HandleAudioFile(inputPath, outputPath); err != nil {
		log.Fatalf("handle audio file: %v", err)
	}

	fmt.Println("demo finished")

	// POST /calls - create call + session and return token
	http.HandleFunc("/calls", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			CallerID string `json:"caller_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		// Generate caller_id if not provided
		if body.CallerID == "" {
			body.CallerID = fmt.Sprintf("user-%d", time.Now().UnixNano())
		}
		callID, sessionID, err := st.CreateCall(body.CallerID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// generate token for caller
		lk := cfg.VendorSettings["livekit"]
		apiKey, apiSecret, url := "", "", ""
		if lk != nil {
			apiKey = lk["api_key"]
			apiSecret = lk["api_secret"]
			url = lk["url"]
		}
		token, err := livekitauth.GenerateAccessToken(apiKey, apiSecret, callID, sessionID, 3600)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp := map[string]string{"call_id": callID, "session_id": sessionID, "token": token, "url": url}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	// LiveKit token endpoint
	http.HandleFunc("/livekit/token", func(w http.ResponseWriter, r *http.Request) {
		room := r.URL.Query().Get("room")
		if room == "" {
			room = "default"
		}
		identity := r.URL.Query().Get("identity")
		if identity == "" {
			identity = "ai-agent"
		}

		// read livekit config from env via config loader
		lk := cfg.VendorSettings["livekit"]
		apiKey := ""
		apiSecret := ""
		url := ""
		if lk != nil {
			apiKey = lk["api_key"]
			apiSecret = lk["api_secret"]
			url = lk["url"]
		}

		token, err := livekitauth.GenerateAccessToken(apiKey, apiSecret, room, identity, 3600)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resp := map[string]string{"url": url, "token": token, "apiKey": apiKey}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	// LiveKit webhook handler - sync participant join/leave
	http.HandleFunc("/webhook/livekit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// read raw body for signature verification and JSON parsing
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		// verify signature if livekit secret is available
		lk := cfg.VendorSettings["livekit"]
		if lk != nil {
			secret := lk["api_secret"]
			if secret != "" {
				sig := r.Header.Get("X-LiveKit-Signature")
				if sig == "" {
					// try alternate header
					sig = r.Header.Get("Livekit-Signature")
				}
				if sig == "" {
					http.Error(w, "missing signature", http.StatusUnauthorized)
					return
				}
				mac := hmac.New(sha256.New, []byte(secret))
				mac.Write(body)
				expected := mac.Sum(nil)

				// compare hex
				hexExp := hex.EncodeToString(expected)
				if hmac.Equal([]byte(hexExp), []byte(sig)) {
					// ok
				} else {
					// compare base64
					b64 := base64.StdEncoding.EncodeToString(expected)
					if !hmac.Equal([]byte(b64), []byte(sig)) {
						http.Error(w, "invalid signature", http.StatusUnauthorized)
						return
					}
				}
			}
		}

		var evt map[string]interface{}
		if err := json.Unmarshal(body, &evt); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		// event types include "participant_joined", "participant_left" etc.
		t, _ := evt["type"].(string)
		// try to extract identity
		identity := ""
		if p, ok := evt["participant"].(map[string]interface{}); ok {
			if id, ok := p["identity"].(string); ok {
				identity = id
			}
		}

		switch t {
		case "participant_joined":
			if identity != "" {
				// mark session active
				_ = st.UpdateSessionStatus(identity, "active")
				// if this participant corresponds to a caller, mark call active
				if callID, _, err := st.FindSessionByIdentity(identity); err == nil {
					_ = st.UpdateCallStatus(callID, "active")
					// Only spawn agent if this is a caller (not the agent itself)
					// Check if this is a caller session by checking session type
					var sessionType string
					row := st.DB.QueryRow(`SELECT type FROM sessions WHERE id = ?`, identity)
					if err := row.Scan(&sessionType); err == nil && sessionType == "caller" {
						// spawn an AI agent for this call (creates session, returns token)
						agentSessionID, token, err := mgr.SpawnAgent(callID)
						if err != nil {
							log.Printf("failed to spawn agent for call %s: %v", callID, err)
						} else {
							log.Printf("spawned agent session=%s token_len=%d for call=%s", agentSessionID, len(token), callID)
						}
					}
				}
			}
		case "participant_left":
			if identity != "" {
				_ = st.UpdateSessionStatus(identity, "ended")
				if callID, _, err := st.FindSessionByIdentity(identity); err == nil {
					// Check if this is the caller leaving - if so, stop agent
					var sessionType string
					row := st.DB.QueryRow(`SELECT type FROM sessions WHERE id = ?`, identity)
					if err := row.Scan(&sessionType); err == nil && sessionType == "caller" {
						// Caller left, stop agent and end call
						if err := mgr.StopAgent(callID); err != nil {
							log.Printf("stop agent error for call %s: %v", callID, err)
						}
						_ = st.UpdateCallStatus(callID, "ended")
					}
				}
			}
		case "room_disconnected", "room_ended":
			// Handle room disconnection - end all calls in this room
			if room, ok := evt["room"].(map[string]interface{}); ok {
				if roomName, ok := room["name"].(string); ok {
					// Find call by room name (callID = room name)
					_ = st.UpdateCallStatus(roomName, "ended")
					// Stop agent if exists
					if err := mgr.StopAgent(roomName); err != nil {
						log.Printf("stop agent error for call %s: %v", roomName, err)
					}
					log.Printf("Room %s disconnected, call ended", roomName)
				}
			}
		default:
			// ignore other events for now
		}

		w.WriteHeader(http.StatusNoContent)
	})

	// Endpoint for external agent interactions under /sessions/{id}/...
	http.HandleFunc("/sessions/", func(w http.ResponseWriter, r *http.Request) {
		// path after prefix should be {id}/<action>
		path := strings.TrimPrefix(r.URL.Path, "/sessions/")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) != 2 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		sessionID := parts[0]
		action := parts[1]

		switch r.Method {
		case http.MethodPost:
			if action != "audio" {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			audio, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "failed to read body", http.StatusBadRequest)
				return
			}
			transcript, err := mgr.ProcessIncomingAudio(sessionID, audio)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"transcript": transcript})
			return

		case http.MethodGet:
			// support GET /sessions/{id}/token to fetch the persisted agent token
			if action != "token" {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}

			// validate optional auth: AGENT_TOKEN_ENDPOINT_SECRET env var
			secret := os.Getenv("AGENT_TOKEN_ENDPOINT_SECRET")
			if secret != "" {
				auth := r.Header.Get("Authorization")
				ok := false
				if strings.HasPrefix(auth, "Bearer ") {
					if strings.TrimPrefix(auth, "Bearer ") == secret {
						ok = true
					}
				}
				if !ok && r.Header.Get("X-Agent-Auth") == secret {
					ok = true
				}
				if !ok {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
			}

			token, err := st.GetSessionToken(sessionID)
			if err != nil {
				http.Error(w, "failed to get token", http.StatusInternalServerError)
				return
			}
			if token == "" {
				http.Error(w, "token not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
			return

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	})

	go func() {
		srvPort := os.Getenv("LIVEKIT_HTTP_PORT")
		if srvPort == "" {
			srvPort = "8080"
		}
		addr := ":" + srvPort
		log.Printf("livekit token server listening on %s", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Fatalf("token server failed: %v", err)
		}
	}()

	// keep the process running so the token server is available
	select {}
}
