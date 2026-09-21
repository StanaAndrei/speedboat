# speedboat

A modern hpc volunteer computing way using go.

## System Architecture & UML Diagrams

This document outlines the struct design, interface implementations, relationships, and method call interactions across the Speedboat platform based on the blueprint in [docs/project.txt](docs/project.txt).

Speedboat decouples scientific orchestrators (**Master Node**) from untrusted volunteer worker nodes (**Agent**) using **NATS** as the message bus and pluggable sandbox runtimes (**Docker**, **Apptainer**, and **WASM**).

---

### 1. Structural UML Diagram (Class Diagram)

The following UML Class Diagram models all Go structs and interfaces across the system, detailing their fields, method receivers, interface realizations (`..|>`), compositions (`*--`), aggregations (`o--`), and dependency associations (`..>`).

```mermaid
classDiagram
    direction TB

    %% ================= Interfaces =================
    class Store {
        <<interface>>
        +SaveRawResult(projectID string, res TaskResult) error
        +GetRawResults(projectID string) TaskResultList
        +SaveVerifiedResult(projectID string, res TaskResult) error
        +GetVerifiedResults(projectID string) TaskResultMap
        +Close() error
    }

    class Engine {
        <<interface>>
        +Name() string
        +IsAvailable() bool
        +PullImage(ctx Context, image string) error
        +Execute(ctx Context, image string, input bytes) bytes
    }

    %% ================= Store Implementation =================
    class BboltStore {
        -bbolt_DB db
        +SaveRawResult(projectID string, res TaskResult) error
        +GetRawResults(projectID string) TaskResultList
        +SaveVerifiedResult(projectID string, res TaskResult) error
        +GetVerifiedResults(projectID string) TaskResultMap
        +Close() error
    }
    Store <|.. BboltStore : implements

    %% ================= Runtime Engine Implementations =================
    class DockerEngine {
        +Name() string
        +IsAvailable() bool
        +PullImage(ctx Context, image string) error
        +Execute(ctx Context, image string, input bytes) bytes
    }

    class ApptainerEngine {
        -string binaryPath
        +Name() string
        +IsAvailable() bool
        +PullImage(ctx Context, image string) error
        +Execute(ctx Context, image string, input bytes) bytes
    }

    class WasmEngine {
        -string binaryPath
        +Name() string
        +IsAvailable() bool
        +PullImage(ctx Context, image string) error
        +Execute(ctx Context, wasmPath string, input bytes) bytes
    }

    Engine <|.. DockerEngine : implements
    Engine <|.. ApptainerEngine : implements
    Engine <|.. WasmEngine : implements

    %% ================= Master / Orchestrator Structs =================
    class Orchestrator {
        +string ProjectDir
        +string NatsURL
        +string Registry
        +bool NoBuild
        +Duration Timeout
        +string DBPath
        +ProjectConfig Config
        +Store Store
        -nats_Conn nc
        -nats_JetStreamContext js
        +Run(ctx Context) error
        -runGenerator(ctx Context, cfg ProjectConfig) RawTaskList
        -collectAndVerifyResults(ctx Context, ...) TaskResultMap
        -runReducer(ctx Context, cfg ProjectConfig, ...) bytes
    }

    class managedTask {
        +string taskID
        +bytes rawBytes
        +Map assignedIPs
        +Map completedIPs
    }

    %% ================= Agent Structs =================
    class Agent {
        +string ProjectID
        +string NatsURL
        +string RuntimePreference
        +string WorkerID
        +string IP
        +int MaxTasks
        +bool Once
        +int IdleTimeout
        -nats_Conn nc
        -nats_JetStreamContext js
        -Engine engine
        +Run(ctx Context) error
        -processTaskMessage(ctx Context, msg nats_Msg) error
        -processTaskData(ctx Context, data bytes) string
    }

    %% ================= Protocol & Schema Structs =================
    class ProjectConfig {
        +string Name
        +string Version
        +ContainerConfig Container
        +ScriptConfig Generator
        +ScriptConfig Reducer
        +VerificationConfig Verification
    }

    class ContainerConfig {
        +string Dockerfile
        +string ImageTag
    }

    class ScriptConfig {
        +string Runtime
        +string Source
    }

    class VerificationConfig {
        +int Redundancy
        +int ConsensusThreshold
    }

    class Task {
        +string TaskID
        +string ProjectID
        +string Image
        +RawMessage Params
        +RawMessage Raw
    }

    class TaskResult {
        +string TaskID
        +string ProjectID
        +string Status
        +RawMessage Output
        +string Error
        +string WorkerID
        +string IP
    }

    class ProjectAnnouncement {
        +string ProjectID
        +string Name
        +string Version
        +string Image
        +string NatsURL
        +int TotalTasks
        +int Redundancy
        +int ConsensusThreshold
        +int64 CreatedAt
    }

    class ReducerBatch {
        +string ProjectID
        +TaskResultList Results
    }

    class TaskRequest {
        +string ProjectID
        +string WorkerID
        +string IP
        +string CompletedTaskID
    }

    class TaskResponse {
        +string Status
        +RawMessage Task
    }

    %% ================= CLI Entrypoint Structs =================
    class MasterCLI {
        +RunConfig Run
        +BuildConfig Build
    }

    class AgentCLI {
        +RunConfig Run
        +InfoConfig Info
    }

    %% ================= Structural Relationships =================
    MasterCLI ..> Orchestrator : instantiates & invokes Run()
    AgentCLI ..> Agent : instantiates & invokes Run()

    Orchestrator *-- ProjectConfig : holds Config
    Orchestrator *-- managedTask : tracks tasks in memory
    Orchestrator o-- Store : delegates persistence via
    Orchestrator ..> BboltStore : initializes default

    ProjectConfig *-- ContainerConfig : embeds
    ProjectConfig *-- ScriptConfig : embeds Generator & Reducer
    ProjectConfig *-- VerificationConfig : embeds

    ReducerBatch o-- TaskResult : aggregates verified

    Agent o-- Engine : executes tasks via
    Agent ..> Task : deserializes
    Agent ..> TaskResult : serializes

    %% ================= Method Calls & Data Flow =================
    Orchestrator ..> Store : calls SaveRawResult(), SaveVerifiedResult(), GetVerifiedResults(), GetRawResults()
    Orchestrator ..> ProjectConfig : calls LoadProjectConfig()
    Agent ..> Engine : calls PullImage() & Execute()

    %% ================= Communication Contracts via NATS =================
    Agent ..> Orchestrator : sends TaskRequest via requests.project_id
    Orchestrator ..> Agent : returns TaskResponse
    Agent ..> Orchestrator : publishes TaskResult via results.project_id
    Orchestrator ..> ReducerBatch : pipes aggregated batch to Reducer
```

---

### 2. Method Call & Dynamic Interaction Flow

The following Sequence Diagram shows how methods across the structs invoke each other during the execution lifecycle—from project initialization and task generation to agent execution, consensus verification, and reduction.

```mermaid
sequenceDiagram
    autonumber
    actor Scientist
    participant MasterCLI as Master CLI (main.go)
    participant Orch as Orchestrator
    participant Store as BboltStore (Store)
    participant NATS as NATS Bus (JetStream)
    participant Agent as Volunteer Agent
    participant Engine as Engine (Docker/Apptainer/WASM)
    actor Volunteer

    Scientist->>MasterCLI: Execute 'master run'
    MasterCLI->>Orch: NewOrchestrator() & Run(ctx)
    activate Orch

    %% Step 1: Config & Build
    Orch->>Orch: LoadProjectConfig(projectDir)
    Orch->>Orch: BuildAndPushImage() (via builder)

    %% Step 2: NATS & Persistence Setup
    Orch->>NATS: Connect() & EnsureTaskStream()
    Orch->>Store: NewBboltStore(dbPath)
    Orch->>Store: GetVerifiedResults(projectID)
    Orch->>Store: GetRawResults(projectID)

    %% Step 3: Task Generation & Queue
    Orch->>Orch: runGenerator(ctx, cfg) [generator.py]
    note over Orch: Populate managedTask pool & reconcile with Store
    Orch->>NATS: Publish ProjectAnnouncement
    Orch->>NATS: Subscribe requests.<project_id>

    %% Volunteer Agent starts up
    Volunteer->>Agent: Execute 'agent run'
    activate Agent
    Agent->>Engine: AutoDetectEngine(preference)
    Agent->>NATS: Connect(natsURL)

    %% Task Request & Execution Loop
    loop Polling & Task Execution Loop
        Agent->>NATS: Request(requests.<project_id>, TaskRequest)
        NATS->>Orch: Dispatch TaskRequest(workerID, ip, completedTaskID)
        activate Orch
        note over Orch: Assign available managedTask to worker IP
        Orch-->>NATS: TaskResponse(status: ok, task: rawBytes)
        deactivate Orch
        NATS-->>Agent: TaskResponse

        Agent->>Agent: processTaskData(ctx, taskBytes)
        activate Agent
        Agent->>Engine: PullImage(ctx, image)
        Agent->>Engine: Execute(ctx, image, stdin=taskBytes)
        activate Engine
        Engine-->>Agent: stdout bytes (computation result)
        deactivate Engine

        Agent->>NATS: Publish(results.<project_id>, TaskResult)
        deactivate Agent

        %% Master verifies consensus
        NATS->>Orch: Handle TaskResult
        activate Orch
        Orch->>Orch: collectAndVerifyResults()
        Orch->>Store: SaveRawResult(projectID, TaskResult)
        note over Orch: Verify Consensus (distinct IPs >= Redundancy & matching >= Threshold)
        opt Consensus Achieved
            Orch->>Store: SaveVerifiedResult(projectID, TaskResult)
        end
        deactivate Orch
    end

    %% Step 4: Reduction
    opt All Tasks Verified
        Orch->>Orch: runReducer(ctx, cfg, verifiedResults) [reducer.py]
        Orch->>Store: Close()
        Orch-->>Scientist: Computation Complete & Display Output
    end

    deactivate Orch
    deactivate Agent
```

---

### 3. Struct Reference & Responsibilities

| Struct / Interface | Package | Key Responsibilities | Relationships & Method Invocations |
| :--- | :--- | :--- | :--- |
| [`Orchestrator`](internal/orchestrator/orchestrator.go#L30-L41) | `internal/orchestrator` | Central master orchestrator. Loads configs, builds images, generates tasks, distributes work, validates consensus, and triggers reduction. | • Owns [`ProjectConfig`](internal/protocol/types.go#L12-L19)<br>• Owns in-memory [`managedTask`](internal/orchestrator/orchestrator.go#L23-L28) pool<br>• Uses [`Store`](internal/store/store.go#L8-L23) (`SaveRawResult`, `SaveVerifiedResult`)<br>• Calls internal `runGenerator`, `collectAndVerifyResults`, `runReducer` |
| [`managedTask`](internal/orchestrator/orchestrator.go#L23-L28) | `internal/orchestrator` | Internal tracking entity for a single task unit, tracking active IP leases and completions. | • Managed exclusively by [`Orchestrator`](internal/orchestrator/orchestrator.go#L30-L41) to enforce redundancy limits and avoid duplicate IP assignment |
| [`Agent`](internal/agent/agent.go#L19-L32) | `internal/agent` | Volunteer client worker. Polls for tasks, executes tasks in a sandbox, and reports results. | • Holds and delegates execution to [`Engine`](internal/runtime/engine.go#L9-L14)<br>• Calls `processTaskData` to parse [`Task`](internal/protocol/types.go#L36-L42) and publish [`TaskResult`](internal/protocol/types.go#L44-L52) |
| [`Engine`](internal/runtime/engine.go#L9-L14) *(Interface)* | `internal/runtime` | Abstraction for isolated execution runtimes (`PullImage`, `Execute`, `IsAvailable`, `Name`). | • Implemented by [`DockerEngine`](internal/runtime/docker.go#L12), [`ApptainerEngine`](internal/runtime/apptainer.go#L11-L13), and [`WasmEngine`](internal/runtime/wasm.go#L11-L13)<br>• Invoked by [`Agent.processTaskData`](internal/agent/agent.go#L194-L273) |
| [`DockerEngine`](internal/runtime/docker.go#L12) | `internal/runtime` | Executes tasks inside rootless Docker containers with `--network none` isolation. | • Implements [`Engine`](internal/runtime/engine.go#L9-L14) |
| [`ApptainerEngine`](internal/runtime/apptainer.go#L11-L13) | `internal/runtime` | Executes tasks inside Apptainer/Singularity HPC containers with network isolation. | • Implements [`Engine`](internal/runtime/engine.go#L9-L14) |
| [`WasmEngine`](internal/runtime/wasm.go#L11-L13) | `internal/runtime` | Executes `.wasm` payloads using `wasmtime`, `wasmer`, or `wazero` pure-Go runtime. | • Implements [`Engine`](internal/runtime/engine.go#L9-L14) |
| [`Store`](internal/store/store.go#L8-L23) *(Interface)* | `internal/store` | Persistence contract for master state recovery (raw worker outputs and verified results). | • Implemented by [`BboltStore`](internal/store/bbolt.go#L21-L23)<br>• Invoked by [`Orchestrator`](internal/orchestrator/orchestrator.go#L30-L41) during startup and verification |
| [`BboltStore`](internal/store/bbolt.go#L21-L23) | `internal/store` | Embedded ACID key-value store implementation using BBolt. | • Implements [`Store`](internal/store/store.go#L8-L23) |
| [`ProjectConfig`](internal/protocol/types.go#L12-L19) | `internal/protocol` | Unmarshaled configuration parsed from `project.yaml`. | • Embeds [`ContainerConfig`](internal/protocol/types.go#L21-L24), [`ScriptConfig`](internal/protocol/types.go#L26-L29), [`VerificationConfig`](internal/protocol/types.go#L31-L34) |
| [`Task`](internal/protocol/types.go#L36-L42) | `internal/protocol` | Unit of scientific computation containing parameters and target container image. | • Generated by generator script, wrapped by master, processed by agent |
| [`TaskResult`](internal/protocol/types.go#L44-L52) | `internal/protocol` | Computation output or error emitted by a worker node for a given task. | • Published by agent to `results.<project_id>`, ingested and persisted by master |
| [`TaskRequest`](internal/protocol/types.go#L71-L76) / [`TaskResponse`](internal/protocol/types.go#L78-L81) | `internal/protocol` | Request-reply payload for work pulling and lease tracking over NATS. | • Exchanged between [`Agent`](internal/agent/agent.go#L19-L32) and [`Orchestrator`](internal/orchestrator/orchestrator.go#L30-L41) on `requests.<project_id>` |
| [`ReducerBatch`](internal/protocol/types.go#L66-L69) | `internal/protocol` | Aggregated slice of consensus-verified results passed to `reducer.py`. | • Constructed by [`Orchestrator.runReducer`](internal/orchestrator/orchestrator.go#L550-L596) |
| [`ProjectAnnouncement`](internal/protocol/types.go#L54-L64) | `internal/protocol` | Broadcast message announcing project availability and metadata. | • Published by [`Orchestrator`](internal/orchestrator/orchestrator.go#L30-L41) on `announcements` and `projects.<id>.info` |
| `CLI` (Master) | `cmd/master` | Command-line interface for the scientist to run the master orchestrator. | • Instantiates and runs [`Orchestrator`](internal/orchestrator/orchestrator.go#L30-L41) |
| `CLI` (Agent) | `cmd/agent` | Command-line interface for volunteers running the compute agent. | • Instantiates and runs [`Agent`](internal/agent/agent.go#L19-L32) |

