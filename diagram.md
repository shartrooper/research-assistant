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