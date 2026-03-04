# Research Assistant

A multi-agent research system built in Go using the [A2A protocol](https://a2a-protocol.org). Given a research topic, it autonomously generates search queries, runs parallel web searches, structures the results with an LLM, writes a report, and answers follow-up questions grounded in the completed research.

---

## Architecture

The system is composed of two A2A agents that communicate via JSON-RPC over HTTP:

```
User / BeeAI
     │  A2A (JSON-RPC)
     ▼
┌─────────────────────┐
│   Concierge (:8080) │  — user-facing agent
│                     │     • accepts research topics
│                     │     • relays live status updates
│                     │     • Q&A mode after research completes
└──────────┬──────────┘
           │  A2A streaming (JSON-RPC)
           ▼
┌─────────────────────┐
│  Researcher (:8081) │  — core research agent
│                     │     • generates queries via Gemini
│                     │     • parallel web search (Google CSE)
│                     │     • structures findings with Gemini
│                     │     • writes report + executive summary
│                     │     • persists artifacts to disk + SQLite
└─────────────────────┘
           │
     ┌─────┴──────┐
     ▼            ▼
 SQLite DB    Disk blobs
(structured)  (report.md,
              report.json)
```

### Agents

**Concierge** (`cmd/concierge`) — the user-facing entry point.
- Receives research topics via A2A task interface
- Calls the Researcher and relays each status update (`searching`, `structuring`, `writing_report`, `completed`) back to the caller in real time
- After research completes, enters **Q&A mode**: subsequent messages on the same A2A context are answered by Gemini, grounded in the persisted key findings and sources for that session

**Researcher** (`cmd/researcher`) — the core research engine.
- Receives a topic via A2A task, generates 3 search queries with Gemini, runs them in parallel, structures the results, and writes a full report + executive summary
- Streams status updates throughout: `searching` → `structuring` → `writing_report` → `completed`
- On completion, persists blobs (report.md, report.json) to disk and structured data (key findings, open questions, sources) to SQLite; returns session identifiers in the final A2A artifact

### Internal pipeline stages

```
topic
  └─▶ Gemini: generate 3 search queries
        └─▶ Google CSE: parallel web search (×3)
              └─▶ Gemini: structure findings into JSON schema
                    └─▶ Gemini: write comprehensive report
                          └─▶ Gemini: executive summary (3–5 bullets)
                                └─▶ persist blobs + SQLite → completed
```

---

## Stack

| Concern | Choice |
|---|---|
| Language | Go 1.24 |
| Agent protocol | [A2A](https://github.com/a2aproject/a2a-go) v0.3.7 (JSON-RPC over HTTP) |
| Orchestrator | BeeAI |
| LLM | Gemini 2.5 Flash (via `google/generative-ai-go`) |
| Search | Google Custom Search Engine (CSE) |
| Database | SQLite (`mattn/go-sqlite3`), upgradeable to PostgreSQL |
| Artifact storage | Disk blobs (MinIO / S3 in production) |

---

## Project layout

```
cmd/
  concierge/      — Concierge A2A agent (port 8080)
  researcher/     — Researcher A2A agent (port 8081)

internal/
  agent/
    concierge/    — Concierge executor + Q&A logic
    researcher/   — Researcher executor (bridges pipeline → A2A)
  pipeline/       — Self-contained research pipeline (LLM + search + persist)
  engine/         — Event engine & session manager (retained for internal use)
  event/          — Event type definitions
  llm/            — Gemini client wrapper
  search/         — Google CSE client
  storage/        — SQLite store + disk blob store
  config/         — Environment variable helpers

data/             — SQLite database (created at runtime)
artifacts/        — Report blobs (created at runtime)
```

---

## Getting started

### Prerequisites

- Go 1.24+
- API keys: Gemini, Google Custom Search (CSE key + CX)

### Environment variables

Copy `.env` and fill in your keys:

```env
GEMINI_API_KEY=...
CSE_API_KEY=...
CSE_CX=...

# Optional — defaults shown
RESEARCHER_ADDR=:8081
CONCIERGE_ADDR=:8080
RESEARCHER_URL=http://localhost:8081
```

### Run

Start both agents (each in its own terminal):

```bash
go run ./cmd/researcher
go run ./cmd/concierge
```

### Send a research request

Using any A2A-compatible client or `curl`:

```bash
curl -s -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": "1",
    "method": "message/send",
    "params": {
      "message": {
        "kind": "message",
        "messageId": "msg-1",
        "role": "user",
        "parts": [{"kind": "text", "text": "The future of Go concurrency"}]
      }
    }
  }'
```

For streaming status updates, use `message/stream` (SSE).

### Agent cards

Each agent exposes its capabilities at:
- `GET http://localhost:8080/.well-known/agent-card.json` — Concierge
- `GET http://localhost:8081/.well-known/agent-card.json` — Researcher

---

## Storage

### Blobs (disk)

`report-<name>-<timestamp>.md` and `.json` are written to `artifacts/` after each completed research session.

### SQLite (`data/research.db`)

| Table | Contents |
|---|---|
| `research_sessions` | One row per topic; tracks status and blob keys |
| `key_findings` | Structured findings with confidence scores |
| `open_questions` | Unresolved questions identified during research |
| `sources` | Web sources (query, URL, snippet) |

---

## Tests

```bash
go test ./...
```

All tests are unit tests with mocked LLM, search, and storage dependencies. Integration tests (requiring API keys) are in `internal/llm` and `internal/search` and are skipped automatically when the relevant environment variables are absent.

---

## Out of scope (v1)

- Queue Manager agent (deferred — Concierge dispatches directly to Researcher)
- MinIO / S3 object storage (DiskBlobStore is the current implementation behind the `BlobStorage` interface)
- Authentication / multi-tenancy
- Per-user rate limiting
- Report versioning