package whisper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/jacky-htg/ai-call-center/libs/interfaces"
)

// whisperSTT calls a local Whisper-like inference HTTP server that accepts a multipart "file" field
// and returns JSON {"text":"..."}.
type whisperSTT struct {
	endpoint string
	client   *http.Client
}

// New constructs a Whisper STT adapter that posts to the default endpoint.
func New() interfaces.STT {
	return NewWithEndpoint("http://localhost:7070/inference")
}

// NewWithEndpoint constructs a Whisper STT adapter using a custom endpoint.
func NewWithEndpoint(endpoint string) interfaces.STT {
	if endpoint == "" {
		endpoint = "http://localhost:7070/inference"
	}
	return &whisperSTT{
		endpoint: endpoint,
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

type whisperResp struct {
	Text string `json:"text"`
}

func (w *whisperSTT) Recognize(audio []byte, opts ...interfaces.STTOption) (string, float32, error) {
	// build multipart form with field name "file"
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	fw, err := mw.CreateFormFile("file", "audio.wav")
	if err != nil {
		return "", 0, fmt.Errorf("create form file: %w", err)
	}
	if _, err := fw.Write(audio); err != nil {
		return "", 0, fmt.Errorf("write audio to form: %w", err)
	}
	// close writer to finalize boundary
	if err := mw.Close(); err != nil {
		return "", 0, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequest("POST", w.endpoint, &b)
	if err != nil {
		return "", 0, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := w.client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("post to whisper server: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, fmt.Errorf("whisper server returned status %d: %s", resp.StatusCode, string(body))
	}

	var wr whisperResp
	if err := json.Unmarshal(body, &wr); err != nil {
		return "", 0, fmt.Errorf("unmarshal response: %w", err)
	}

	// The local server returned plain transcript. Confidence isn't provided, return 1.0 by default.
	return wr.Text, 1.0, nil
}
