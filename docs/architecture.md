# Architecture - AI Call Center (overview)

This document explains the high-level architecture and how the codebase is organized.

Core concepts

- Interfaces-first: `pkg/interfaces` defines small, focused interfaces for TTS, STT, LLM and WebRTC. This keeps business logic independent from vendor SDKs.
- Vendor adapters: each provider implementation lives under `internal/vendors/<vendor>` and implements the interfaces.
- Agent orchestration: `internal/agents` holds the code that coordinates the streaming flow: receive audio -> STT -> LLM -> TTS -> output stream.
- WebRTC transport: `internal/vendors/livekit` should provide the glue for connecting browser/phone clients using LiveKit.

Suggested flow for a call

1. Web client connects through LiveKit (signaling). LiveKit session is started via `WebRTCProvider`.
2. Audio frames are forwarded to an STT implementation (Piper initially).
3. Transcripts are fed to the LLM (Ollama initially) to determine responses or actions.
4. Responses are converted to audio through a TTS implementation (Whisper initially), then streamed back to the caller.

Notes on swapping vendors

- Keep adapter constructors small and return the interface type. Use a factory that accepts vendor key strings.
- Vendor-specific options should be stored in `internal/config.Config.VendorSettings`.
