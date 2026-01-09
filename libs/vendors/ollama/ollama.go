package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jacky-htg/ai-call-center/libs/interfaces"
)

type ollamaLLM struct {
	endpoint string
	model    string
	client   *http.Client
}

// New returns a client configured for the local Ollama HTTP API.
func New() interfaces.LLM {
	return NewWithEndpointModel("http://localhost:11434/api/generate", "tinyllama")
}

// NewWithEndpointModel creates an Ollama client with custom endpoint and model.
func NewWithEndpointModel(endpoint, model string) interfaces.LLM {
	if endpoint == "" {
		endpoint = "http://localhost:11434/api/generate"
	}
	if model == "" {
		model = "tinyllama"
	}
	return &ollamaLLM{endpoint: endpoint, model: model, client: &http.Client{Timeout: 30 * time.Second}}
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	Response  string `json:"response"`
	Done      bool   `json:"done"`
}

func (o *ollamaLLM) Generate(prompt string, opts ...interfaces.LLMOption) (string, error) {
	reqBody := ollamaRequest{Model: o.model, Prompt: prompt, Stream: false}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal ollama request: %w", err)
	}

	resp, err := o.client.Post(o.endpoint, "application/json", bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("post to ollama: %w", err)
	}
	defer resp.Body.Close()

	var out ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}

	return out.Response, nil
}
