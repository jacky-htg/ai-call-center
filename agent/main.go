package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

func mustEnv(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func main() {
	server := mustEnv("SERVER_URL", "http://localhost:8080")
	dbPath := mustEnv("DATABASE_PATH", "data/ai.callcenter.db")

	// 1) create a call
	reqBody := map[string]string{"caller_id": "external-caller"}
	b, _ := json.Marshal(reqBody)
	resp, err := http.Post(server+"/calls", "application/json", bytes.NewReader(b))
	if err != nil {
		log.Fatalf("create call failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("create call status: %d body: %s", resp.StatusCode, string(body))
	}
	var r map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		log.Fatalf("decode create response: %v", err)
	}
	callID := r["call_id"]
	callerSession := r["session_id"]
	fmt.Printf("created call %s caller_session %s\n", callID, callerSession)

	// 2) simulate participant_joined webhook so server will spawn agent
	webhook := map[string]any{"type": "participant_joined", "participant": map[string]string{"identity": callerSession}}
	wb, _ := json.Marshal(webhook)
	wresp, err := http.Post(server+"/webhook/livekit", "application/json", bytes.NewReader(wb))
	if err != nil {
		log.Fatalf("webhook post failed: %v", err)
	}
	wresp.Body.Close()
	if wresp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(wresp.Body)
		log.Fatalf("webhook status: %d body: %s", wresp.StatusCode, string(body))
	}
	fmt.Println("sent participant_joined webhook")

	// wait a moment for the server to spawn agent and persist token
	time.Sleep(500 * time.Millisecond)

	// 3) open DB and find agent session for our call
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var agentSessionID, token string
	row := db.QueryRow("SELECT id, token FROM sessions WHERE call_id = ? AND type = 'agent' ORDER BY created_at DESC LIMIT 1", callID)
	if err := row.Scan(&agentSessionID, &token); err != nil {
		log.Fatalf("find agent session: %v", err)
	}
	fmt.Printf("found agent session %s token_len=%d\n", agentSessionID, len(token))

	// 4) post audio to server so it will run STT->LLM->TTS
	audioPath := "testdata/jfk.wav"
	f, err := os.Open(audioPath)
	if err != nil {
		log.Fatalf("open audio: %v", err)
	}
	defer f.Close()
	aud, err := io.ReadAll(f)
	if err != nil {
		log.Fatalf("read audio: %v", err)
	}

	postURL := fmt.Sprintf("%s/sessions/%s/audio", server, agentSessionID)
	req, err := http.NewRequest(http.MethodPost, postURL, bytes.NewReader(aud))
	if err != nil {
		log.Fatalf("new req: %v", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("post audio failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		log.Fatalf("post audio status: %d body: %s", resp2.StatusCode, string(body))
	}
	var ar map[string]string
	if err := json.NewDecoder(resp2.Body).Decode(&ar); err != nil {
		log.Fatalf("decode audio response: %v", err)
	}
	fmt.Printf("transcript: %s\n", ar["transcript"])

	fmt.Println("external agent run complete")
}
