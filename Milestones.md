# Event-Driven Research Assistant Agent — Development Phases

This document describes a learn-by-building progression for constructing a fully event-driven research assistant agent in Go.
Each phase introduces foundational event-driven development (EDD) concepts through incremental hands-on implementation.

## Phase 0 — Minimal Event Loop (System Heartbeat) COMPLETED

Goal

Build the simplest possible event-driven program: an event queue, an event loop, and a dispatcher.

Concepts Introduced

Event queue

Basic event structure

Core event loop

Dispatch function

Outcome

A Go program that processes events one at a time.

Milestone Code Concepts
type Event struct {
Type EventType
Data any
}

var EventQueue = make(chan Event, 32)

func mainLoop() {
for {
ev := <-EventQueue
dispatch(ev)
}
}

## Phase 1 — Single Event Chain (Fundamental Dispatching) COMPLETED

Goal

Introduce event chaining: a handler emits new events.

Concepts Introduced

Single event → next event

Internal generation of events

Basic decoupling

Outcome

A message in leads to a second stage event.

Milestone Code Concepts
case EventUserInput:
EventQueue <- Event{EventGenerateReply, "Processing..."}

## Phase 2 — Multi-Stage Pipeline (Event Sequencing) COMPLETED

Goal

Implement a 3-step research pipeline using chained events.

Pipeline

UserInputReceived

AnalysisRequested

SummaryRequested

Concepts Introduced

Multi-event workflows

Event choreography

Pipeline design

Outcome

A fully functional event-driven sequence.

Milestone Code Concepts
UserInputReceived → AnalysisRequested → SummaryRequested → SummaryComplete

## Phase 3 — Add Finite State Machine (FSM) COMPLETED

Goal

Introduce application-level state to regulate event handling.

Concepts Introduced

FSM design

State transitions

Guard conditions

Event gating

States
IDLE → WAITING → ANALYZING → SUMMARIZING → RESPONDING

Outcome

The agent behaves more realistically and can avoid invalid transitions.

Milestone Code Concepts
if currentState != StateIdle && ev.Type == EventUserInput {
// reject or buffer input
}

## Phase 4 — Parallel Workers (Concurrency & Fan-Out) COMPLETED

Goal

Introduce parallelism using goroutines and worker pools.

Concepts Introduced

Fan-in and fan-out

Worker pools

Backpressure

Queue design

Concurrency-safe event handling

Outcome

Your agent can parallelize analysis or external tasks.

Milestone Code Concepts
for i := 0; i < 4; i++ {
go searchWorker(i)
}

## Phase 5 — Timers, Timeouts & Scheduled Events COMPLETED

Goal

Incorporate time as a first-class event source.

Concepts Introduced

Timer-driven events

Heartbeat/tick events

Operation timeouts

Error and retry events

Outcome

The system becomes reactive to time; operations can expire or retry.

Milestone Code Concepts
go func() {
for {
time.Sleep(time.Second)
EventQueue <- Event{EventTick, nil}
}
}()

## Phase 6 — Tool Integrations (External Events)

Goal

Integrate real external APIs and tools into the event-driven pipeline.

Concepts Introduced

Asynchronous I/O

Tool events (start, done, error)

Workflow orchestration

Service decoupling

Tools You May Integrate

Web search API

HTTP data fetchers

LLM API

Local ML models

Data scrapers

Outcome

The agent transitions from local toy to real research assistant.

Milestone Code Concepts
go func() {
results, err := doSearch(query)
if err != nil {
EventQueue <- Event{EventError, err}
return
}
EventQueue <- Event{EventSearchCompleted, results}
}()

## Phase 7 — Artifact Generation System (Documents, Charts, Files)

Goal

Add fully asynchronous generation of artifacts:

Documents

Charts

Images

Files (JSON, CSV)

Concepts Introduced

Task-specific worker modules

Event-driven document creation

Chart/graph rendering

File management

Artifact registry

Event-driven error handling

New Artifact Events
ArtifactRequested
ArtifactGenerated
DocumentReady
ChartReady
FileSaved
ArtifactError

Outcome

Your agent produces tangible research outputs — documents, charts, visualizations — making it a real-world system.

Milestone Code Concepts
EventQueue <- Event{
Type: ArtifactRequested,
Data: ArtifactSpec{
Type: "chart",
Payload: chartData,
},
}

## Final Deliverable — A Full Event-Driven Research Agent

After completing all phases, you will have built:

✔ A true event-driven system
✔ A modular dispatching architecture
✔ A complete FSM
✔ Parallel worker pools
✔ Tool-integrated workflows
✔ Time-based scheduling
✔ Artifact creation & output system
✔ A scalable, extensible research agent

This system integrates every major EDD principle, making the project both a learning experience and a professional-grade architecture.