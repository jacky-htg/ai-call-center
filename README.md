AI Call Center - pluggable vendor architecture

This repository contains a starting structure for an AI-powered call center where
voice and assistant components are implemented by swappable vendor adapters.

Key ideas:
- All vendor implementations must conform to the interfaces in `pkg/interfaces`.
- Runtime selection of vendors is through `internal/config.Config`.
- Initial vendor choices (example): TTS: Whisper, STT: Piper, LLM: Ollama, WebRTC: LiveKit

Repository layout (high level):

- `cmd/server` — application entrypoint(s).
- `internal/agents` — call orchestration logic (CallAgent, session management).
- `internal/config` — configuration and vendor selection.
- `internal/vendors/*` — vendor adapter folders (whisper, piper, ollama, livekit, ...).
- `pkg/interfaces` — stable interfaces for TTS/STT/LLM/WebRTC.
- `docs` — architecture and design notes.

How to add/swap a vendor
1. Implement the relevant interface from `pkg/interfaces` in a folder under `internal/vendors`.
2. Wire the implementation creation in startup (for example, an adapter factory using `internal/config`).
3. Use dependency injection to provide the concrete implementation to `internal/agents.CallAgent`.

Next steps (suggested):
- Add adapter implementations for `internal/vendors/whisper`, `piper`, `ollama`, `livekit`.
- Implement a simple DI/factory that reads `internal/config.Config` to select implementations.
- Add unit tests for interface contracts and a small integration harness.
