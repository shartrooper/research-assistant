## Event-Driven Research Assistant — Task Lifecycle

```mermaid
flowchart TD
  A["UserInputReceived"] --> B["Gemini: generate search queries (JSON array)"]
  B --> C["Create ResearchSession (SessionID, expected N queries)"]
  C --> D["SearchRequested (xN)"]
  D --> E["Search Engine Workers (parallel)"]
  E --> F["SearchCompleted (xN)"]
  F --> G["Collector: aggregate results per session"]
  G --> H["AnalysisRequested (SearchAggregate)"]
  H --> I["Gemini: structure results into JSON schema"]
  I --> J{"Structured JSON valid?"}
  J -- "No: parse/format error" --> I1["Fallback StructuredResearch (error/open_questions)"]
  J -- "Yes" --> K["StructuredDataReady"]
  I1 --> K

  K --> L{"Structured error present?"}
  L -- "Yes" --> L1["SummaryRequested (error message)"]
  L -- "No" --> M["Gemini: write report from structured data"]
  M --> N["Gemini: executive summary (3-5 bullets)"]
  N --> O["SummaryRequested"]
  O --> P["SummaryComplete"]
  L1 --> P
  P --> Q["Artifacts: report.md, report.json, structured.json, sources.csv"]

  E -.-> X["Search failure (record error, continue)"]
  X --> F
  B -.-> Y["Query generation error (fallback: single query)"]
  Y --> C
  M -.-> Z["Report/summary error (safe message)"]
  Z --> P
```

## Event Lifecycle by Workers (Sequence)

```mermaid
sequenceDiagram
    actor User
    participant Engine as Event Engine
    participant Worker as Worker Pool (xN)
    participant Session as Session Manager
    participant LLM as Gemini API
    participant Search as Search Engine API
    participant Artifacts as Artifact Writer

    User->>Engine: Publish UserInputReceived
    Engine->>Worker: Dispatch UserInputReceived
    Worker->>LLM: Generate queries (JSON array)

    alt Query generation fails
        LLM-->>Worker: Error
        Worker->>Worker: Fallback to single query (user's prompt)
    else Query generation succeeds
        LLM-->>Worker: Queries [q1, q2, ...]
    end

    Worker->>Session: CreateSession(sessionID, expectedQueries)
    Worker->>Engine: Publish SearchRequested (xN)

    par Parallel search tasks
        Engine->>Worker: Dispatch SearchRequested(q1)
        Worker->>Search: Search(q1)
        Search-->>Worker: Result/Error
        Worker->>Engine: Publish SearchCompleted
    and
        Engine->>Worker: Dispatch SearchRequested(q2)
        Worker->>Search: Search(q2)
        Search-->>Worker: Result/Error
        Worker->>Engine: Publish SearchCompleted
    and
        Engine->>Worker: Dispatch SearchRequested(qN)
        Worker->>Search: Search(qN)
        Search-->>Worker: Result/Error
        Worker->>Engine: Publish SearchCompleted
    end

    loop Until all expected results are accounted for
        Engine->>Worker: Dispatch SearchCompleted
        Worker->>Session: AddResult(sessionID)
        alt Session complete
            Session-->>Worker: true
            Worker->>Engine: Publish AnalysisRequested(SearchAggregate)
            Worker->>Session: DeleteSession(sessionID)
        else Still waiting
            Session-->>Worker: false
        end
    end

    Engine->>Worker: Dispatch AnalysisRequested
    Worker->>LLM: Structure raw sources into schema JSON
    alt Invalid JSON or parse failure
        LLM-->>Worker: Malformed/invalid output
        Worker->>Worker: Build fallback StructuredResearch
    else Valid schema
        LLM-->>Worker: StructuredResearch JSON
    end
    Worker->>Engine: Publish StructuredDataReady

    Engine->>Worker: Dispatch StructuredDataReady
    alt StructuredResearch.error is set
        Worker->>Engine: Publish SummaryRequested(error message)
    else Structured data is usable
        Worker->>LLM: Generate report from structured data
        LLM-->>Worker: Report
        Worker->>LLM: Generate executive summary
        LLM-->>Worker: Summary bullets
        Worker->>Engine: Publish SummaryRequested(report+summary)
    end

    Engine->>Worker: Dispatch SummaryRequested
    Worker->>Engine: Publish SummaryComplete

    Engine->>Worker: Dispatch SummaryComplete
    Worker->>Artifacts: Write report.md, report.json, structured.json, sources.csv
    Artifacts-->>Worker: Output path / error
```

## Legend (Mermaid Sequence)

- `A ->> B`: synchronous message/request from A to B
- `A -->> B`: return/response from A to B
- `alt / else`: conditional branch (if/else)
- `par / and / end`: parallel branches executing concurrently
- `loop`: repeated block until condition is met
- `participant`: system component/lifeline in the interaction
- `actor`: external initiator (e.g., user)
- `Publish <EventType>`: enqueue event into the event engine
- `Dispatch <EventType>`: worker pool picks and handles queued event