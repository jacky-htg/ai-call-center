package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	var (
		sessionID  string
		backendURL string
		timeoutSec int
	)
	flag.StringVar(&sessionID, "session", "", "agent session ID to fetch token for")
	flag.StringVar(&backendURL, "backend", "http://localhost:8080", "backend base URL")
	flag.IntVar(&timeoutSec, "timeout", 10, "HTTP timeout seconds")
	flag.Parse()

	if sessionID == "" {
		fmt.Fprintln(os.Stderr, "session id required: -session <id>")
		os.Exit(2)
	}

	// build token endpoint
	tokenEndpoint := fmt.Sprintf("%s/sessions/%s/token", backendURL, sessionID)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", tokenEndpoint, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "new request: %v\n", err)
		os.Exit(1)
	}

	// optional auth using AGENT_TOKEN_ENDPOINT_SECRET
	if s := os.Getenv("AGENT_TOKEN_ENDPOINT_SECRET"); s != "" {
		req.Header.Set("Authorization", "Bearer "+s)
	}

	client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "request failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "bad status %d: %s\n", resp.StatusCode, string(b))
		os.Exit(1)
	}

	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		fmt.Fprintf(os.Stderr, "decode response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("session=%s token=%s\n", sessionID, out.Token)

	// Attempt to open websocket signaling to LiveKit /rtc endpoint using the retrieved token
	// Build WS URL: <LIVEKIT_URL>/rtc?access_token=<token>
	livekitURL := os.Getenv("LIVEKIT_URL")
	if livekitURL == "" {
		fmt.Fprintln(os.Stderr, "LIVEKIT_URL not set in env; skipping WS join")
		return
	}

	// ensure token is present
	if out.Token == "" {
		fmt.Fprintln(os.Stderr, "no token returned; cannot join LiveKit")
		return
	}

	// parse livekitURL and append path /rtc and query
	u, err := url.Parse(livekitURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid LIVEKIT_URL: %v\n", err)
		return
	}
	// ensure scheme is ws/wss
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else if u.Scheme == "http" {
		u.Scheme = "ws"
	}
	u.Path = "/rtc"
	q := u.Query()
	q.Set("access_token", out.Token)
	u.RawQuery = q.Encode()

	fmt.Printf("dialing livekit websocket %s\n", u.String())
	dialer := websocket.DefaultDialer
	// create connection
	wsConn, resp2, err := dialer.Dial(u.String(), nil)
	if err != nil {
		if resp2 != nil {
			b, _ := io.ReadAll(resp2.Body)
			fmt.Fprintf(os.Stderr, "websocket dial failed: %v status=%d body=%s\n", err, resp2.StatusCode, string(b))
		} else {
			fmt.Fprintf(os.Stderr, "websocket dial failed: %v\n", err)
		}
		return
	}
	defer wsConn.Close()

	// read messages in background
	go func() {
		for {
			mt, msg, err := wsConn.ReadMessage()
			if err != nil {
				fmt.Fprintf(os.Stderr, "ws read error: %v\n", err)
				return
			}
			fmt.Printf("ws message (type=%d): %s\n", mt, string(msg))
		}
	}()

	// keep open for a short duration so we can observe signaling
	fmt.Println("connected to LiveKit signaling websocket; keeping connection open for 20s to observe messages")
	time.Sleep(20 * time.Second)
	fmt.Println("done")
}
