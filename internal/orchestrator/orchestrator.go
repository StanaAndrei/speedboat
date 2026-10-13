package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"speedboat/internal/builder"
	"speedboat/internal/protocol"
	"speedboat/internal/runtime"
	"speedboat/internal/store"
	"speedboat/internal/transport"

	"github.com/nats-io/nats.go"
)

type Orchestrator struct {
	ProjectDir     string
	NatsURL        string
	Registry       string
	NoBuild        bool
	Timeout        time.Duration
	DBPath         string
	SendDup        bool
	Config         *protocol.ProjectConfig
	nc             *nats.Conn
	js             nats.JetStreamContext
	Store          store.Store
	blacklistMu    sync.RWMutex
	blacklistedIPs map[string]bool
	Engine         runtime.Engine
	WasmPath       string
	WebPort        int
	NatsWsURL      string
	httpServer     *http.Server
}

func NewOrchestrator(projectDir string, natsURL string, registry string, noBuild bool, dbPath string, sendDup ...bool) *Orchestrator {
	absDir, err := filepath.Abs(projectDir)
	if err != nil {
		absDir = projectDir
	}
	db := dbPath
	if db == "" {
		db = "speedboat.db"
	}
	isSendDup := false
	if len(sendDup) > 0 {
		isSendDup = sendDup[0]
	}
	return &Orchestrator{
		ProjectDir:     absDir,
		NatsURL:        natsURL,
		Registry:       registry,
		NoBuild:        noBuild,
		Timeout:        0, // unlimited by default
		DBPath:         db,
		SendDup:        isSendDup,
		WebPort:        3000,
		blacklistedIPs: make(map[string]bool),
	}
}

func (o *Orchestrator) Run(ctx context.Context) error {
	// 1. Load project config
	cfg, err := protocol.LoadProjectConfig(o.ProjectDir)
	if err != nil {
		return fmt.Errorf("failed to load project config: %w", err)
	}
	o.Config = cfg

	fmt.Printf("🚀 Speedboat Master: Project '%s' (v%s)\n", cfg.Name, cfg.Version)
	fmt.Printf("   Redundancy: %d | Consensus Threshold: %d | Iterations: %d | Send Duplicate: %v\n",
		cfg.Verification.Redundancy, cfg.Verification.ConsensusThreshold, cfg.Verification.Iterations, o.SendDup)

	// 2. Build & push container image
	var imageTag string
	if !o.NoBuild {
		fmt.Printf("📦 Step 1/6: Baking Docker image from %s...\n", cfg.Container.Dockerfile)
		tag, err := builder.BuildAndPushImage(ctx, o.ProjectDir, cfg.Container, o.Registry)
		if err != nil {
			return fmt.Errorf("image baking failed: %w", err)
		}
		imageTag = tag
		fmt.Printf("   ✓ Successfully baked and pushed image: %s\n", imageTag)
	} else {
		if o.Registry != "" {
			imageTag = fmt.Sprintf("%s/%s", o.Registry, cfg.Container.ImageTag)
		} else {
			imageTag = cfg.Container.ImageTag
		}
		fmt.Printf("⏩ Step 1/6: Skipping build. Using image tag: %s\n", imageTag)
	}

	// 3. Connect to NATS + setup JetStream
	fmt.Printf("🔌 Step 2/6: Connecting to NATS at %s...\n", o.NatsURL)
	nc, js, err := transport.Connect(o.NatsURL)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}
	defer nc.Close()
	o.nc = nc
	o.js = js

	fmt.Printf("   ✓ Initializing JetStream WorkQueue stream for project '%s'...\n", cfg.Name)
	if _, err := transport.EnsureTaskStream(js, cfg.Name); err != nil {
		return fmt.Errorf("failed to ensure task stream: %w", err)
	}

	// 4. Initialize persistent store
	if o.Store == nil {
		fmt.Printf("💾 Initializing persistent store at %s...\n", o.DBPath)
		dbStore, err := store.NewSQLStore(o.DBPath)
		if err != nil {
			return fmt.Errorf("failed to initialize store at %s: %w", o.DBPath, err)
		}
		defer dbStore.Close()
		o.Store = dbStore
	}

	if o.Store != nil {
		persistedBlacklist, err := o.Store.GetBlacklistedIPs()
		if err != nil {
			fmt.Printf("⚠️  [Master] Failed to load blacklisted IPs from store: %v\n", err)
		} else {
			o.blacklistMu.Lock()
			for _, ip := range persistedBlacklist {
				o.blacklistedIPs[ip] = true
			}
			o.blacklistMu.Unlock()
			if len(persistedBlacklist) > 0 {
				fmt.Printf("   🚫 Loaded %d blacklisted IPs from database\n", len(persistedBlacklist))
			}
		}
	}

	// 5. Run task generator
	fmt.Printf("⚙️  Step 3/6: Running generator script (%s %s)...\n", cfg.Generator.Runtime, cfg.Generator.Source)
	rawTasks, err := o.runGenerator(ctx, cfg)
	if err != nil {
		return fmt.Errorf("generator script failed: %w", err)
	}
	totalTasks := len(rawTasks)
	if totalTasks == 0 {
		return fmt.Errorf("generator produced 0 tasks")
	}
	fmt.Printf("   ✓ Generated %d unique tasks\n", totalTasks)

	// 6. Publish announcement & info subscription
	fmt.Printf("📢 Step 4/6: Publishing project announcement and queuing tasks...\n")
	announcement := protocol.ProjectAnnouncement{
		ProjectID:          cfg.Name,
		Name:               cfg.Name,
		Version:            cfg.Version,
		Image:              imageTag,
		NatsURL:            o.NatsURL,
		TotalTasks:         totalTasks,
		Redundancy:         cfg.Verification.Redundancy,
		ConsensusThreshold: cfg.Verification.ConsensusThreshold,
		Iterations:         cfg.Verification.Iterations,
		CreatedAt:          time.Now().Unix(),
	}
	annBytes, _ := json.Marshal(announcement)

	infoSub, _ := nc.Subscribe(transport.ProjectInfoSubject(cfg.Name), func(msg *nats.Msg) {
		if msg.Reply != "" {
			_ = msg.Respond(annBytes)
		}
	})
	defer func() {
		if infoSub != nil {
			_ = infoSub.Unsubscribe()
		}
	}()

	// Start browser volunteer web server if --wasm payload is configured
	cleanupWeb, err := o.startVolunteerServer()
	if err != nil {
		return err
	}
	defer cleanupWeb()

	// 7. Load persisted state and reconcile task pool
	persistedVerified, err := o.Store.GetVerifiedResults(cfg.Name)
	if err != nil {
		return fmt.Errorf("failed to load verified results from store: %w", err)
	}
	persistedRaw, err := o.Store.GetRawResults(cfg.Name)
	if err != nil {
		return fmt.Errorf("failed to load raw results from store: %w", err)
	}

	reqRedundancy := cfg.Verification.Redundancy
	if reqRedundancy <= 0 {
		reqRedundancy = 1
	}
	threshold := cfg.Verification.ConsensusThreshold
	if threshold <= 0 {
		threshold = 1
	}
	if threshold > reqRedundancy {
		threshold = reqRedundancy
	}

	taskState, err := reconcileTasksFromStore(
		rawTasks,
		cfg.Name,
		imageTag,
		reqRedundancy,
		threshold,
		o.SendDup,
		persistedVerified,
		persistedRaw,
		o.Store.SaveVerifiedResult,
	)
	if err != nil {
		return err
	}

	tasksList := taskState.TasksList
	tasksMap := taskState.TasksMap
	taskOrder := taskState.TaskOrder
	verifiedResults := taskState.VerifiedResults
	taskOutputs := taskState.TaskOutputs
	taskIPs := taskState.TaskIPs

	if len(verifiedResults) > 0 {
		fmt.Printf("   💾 Restored from DB: %d/%d tasks already verified.\n", len(verifiedResults), totalTasks)
	}
	fmt.Printf("   ✓ Initialized dispatcher for %d remaining tasks to process (redundancy %d)\n",
		len(tasksList), cfg.Verification.Redundancy)

	rand.Shuffle(len(tasksList), func(i, j int) {
		tasksList[i], tasksList[j] = tasksList[j], tasksList[i]
	})

	var (
		taskMu sync.Mutex
		isDone bool
	)
	if len(verifiedResults) >= totalTasks {
		isDone = true
	}

	// 8. Work Pulling Subscription (Task Dispatcher)
	reqSub, err := nc.Subscribe(transport.TaskRequestSubject(cfg.Name), func(msg *nats.Msg) {
		var req protocol.TaskRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			fmt.Printf("⚠️  [Master] Received invalid task request: %v\n", err)
			return
		}

		workerIP := getWorkerIP(req.IP, req.WorkerID)
		if o.isIPBlacklisted(workerIP) {
			respBytes, _ := json.Marshal(protocol.TaskResponse{Status: "blacklisted"})
			_ = msg.Respond(respBytes)
			return
		}

		workerKey := workerIP
		if o.SendDup && req.WorkerID != "" {
			workerKey = req.WorkerID
		}

		taskMu.Lock()
		defer taskMu.Unlock()

		if req.CompletedTaskID != "" {
			if mt, ok := tasksMap[req.CompletedTaskID]; ok {
				if _, assigned := mt.assignedIPs[workerKey]; assigned {
					delete(mt.assignedIPs, workerKey)
					mt.completedCount++
				}
				mt.completedIPs[workerIP] = true
			}
		}

		if isDone {
			respBytes, _ := json.Marshal(protocol.TaskResponse{Status: "completed"})
			_ = msg.Respond(respBytes)
			return
		}

		// Clear any existing in-flight assignment for this worker
		for _, mt := range tasksList {
			delete(mt.assignedIPs, workerKey)
		}

		selected := selectTaskForWorker(tasksList, verifiedResults, workerIP, workerKey, o.SendDup, cfg.Verification.Redundancy)

		if selected != nil {
			selected.assignedIPs[workerKey] = time.Now()
			if _, isVerified := verifiedResults[selected.taskID]; isVerified {
				fmt.Printf("   🪤 Dispatched task %s as TRAP to worker %s (IP: %s)\n",
					selected.taskID, req.WorkerID, workerIP)
			}
			respBytes, _ := json.Marshal(protocol.TaskResponse{
				Status: "ok",
				Task:   selected.rawBytes,
			})
			_ = msg.Respond(respBytes)
			return
		}

		respBytes, _ := json.Marshal(protocol.TaskResponse{Status: "no_tasks"})
		_ = msg.Respond(respBytes)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to task requests: %w", err)
	}
	defer reqSub.Unsubscribe()

	// 9. Collect Results & Consensus Verification
	fmt.Printf("🔍 Step 5/6: Waiting for volunteer results on '%s' & verifying consensus...\n", transport.ResultsSubject(cfg.Name))
	verifiedResults, err = o.collectAndVerifyResults(
		ctx,
		cfg,
		taskOrder,
		&taskMu,
		tasksMap,
		&isDone,
		verifiedResults,
		taskOutputs,
		taskIPs,
	)

	if err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}

	// 10. Pipe Verified Results to Reducer
	fmt.Printf("📊 Step 6/6: Piping verified batch to reducer (%s %s)...\n", cfg.Reducer.Runtime, cfg.Reducer.Source)
	reducerOutput, err := o.runReducer(ctx, cfg, taskOrder, verifiedResults)
	if err != nil {
		return fmt.Errorf("reducer script failed: %w", err)
	}

	fmt.Println("\n🎉 Computation Complete! Reducer Output:")
	fmt.Println(string(reducerOutput))

	return nil
}
