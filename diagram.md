## Event-Driven Research Assistant — Mermaid Overview

```mermaid
flowchart TD
  P0["Phase 0: Minimal Event Loop - Event queue - Basic event structure - Core event loop - Dispatcher"]
  P1["Phase 1: Single Event Chain - Handlers emit new events - Single event → next event - Internal event generation - Basic decoupling"]
  P2["Phase 2: Multi-Stage Pipeline - 3-step research pipeline - UserInputReceived → AnalysisRequested → SummaryRequested - Multi-event workflows - Event choreography - Pipeline design"]
  P3["Phase 3: Finite State Machine (FSM) - FSM design - State transitions - Guard conditions - Event gating - IDLE → WAITING → ANALYZING → SUMMARIZING → RESPONDING"]
  P4["Phase 4: Parallel Workers - Parallelism with workers - Fan-in / fan-out - Worker pools - Backpressure - Queue design - Concurrency-safe handling"]
  P5["Phase 5: Time-Based Events - Timer-driven events - Heartbeat/tick events - Timeouts - Error & retry events"]
  P6["Phase 6: Tool Integrations - Asynchronous I/O - Tool start/done/error events - Workflow orchestration - Service decoupling - External tools & APIs"]
  P7["Phase 7: Artifact Generation - Task-specific workers - Event-driven document creation - Charts & visualizations - File outputs (JSON, CSV, etc.) - Artifact registry - Artifact error handling"]
  F["Final Deliverable - Full event-driven system - Modular dispatching - Complete FSM - Parallel workers - Tool-integrated workflows - Time-based scheduling - Artifact creation & outputs - Scalable research agent"]

  P0 --> P1 --> P2 --> P3 --> P4 --> P5 --> P6 --> P7 --> F
```


