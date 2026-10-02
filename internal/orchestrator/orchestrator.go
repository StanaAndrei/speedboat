package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"speedboat/internal/builder"
	"speedboat/internal/protocol"
	"speedboat/internal/runtime"
	"speedboat/internal/store"
	"speedboat/internal/transport"

	"github.com/nats-io/nats.go"
)

type managedTask struct {
	taskID         string
	rawBytes       []byte
	assignedIPs    map[string]time.Time
	completedIPs   map[string]bool
	completedCount int
	iteration      int
}

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
		blacklistedIPs: make(map[string]bool),
	}
}

func (o *Orchestrator) isIPBlacklisted(ip string) bool {
	if ip == "" {
		return false
	}
	o.blacklistMu.RLock()
	defer o.blacklistMu.RUnlock()
	return o.blacklistedIPs[ip]
}

func (o *Orchestrator) blacklistIP(ip, reason string, tasksMap ...map[string]*managedTask) {
	if ip == "" {
		return
	}
	o.blacklistMu.Lock()
	if o.blacklistedIPs == nil {
		o.blacklistedIPs = make(map[string]bool)
	}
	already := o.blacklistedIPs[ip]
	o.blacklistedIPs[ip] = true
	o.blacklistMu.Unlock()

	if len(tasksMap) > 0 && tasksMap[0] != nil {
		for _, mt := range tasksMap[0] {
			delete(mt.assignedIPs, ip)
		}
	}

	if !already {
		fmt.Printf("🚨 [Master] Blacklisted IP %s: %s\n", ip, reason)
		if o.Store != nil {
			if err := o.Store.BlacklistIP(ip, reason); err != nil {
				fmt.Printf("⚠️  [Master] Failed to persist blacklisted IP %s: %v\n", ip, err)
			}
		}
	}
}

func (o *Orchestrator) executeTaskLocally(ctx context.Context, taskBytes []byte, fallbackImage string) (protocol.TaskResult, error) {
	if o.Engine == nil {
		o.Engine = runtime.NewDockerEngine()
	}

	var task protocol.Task
	if err := json.Unmarshal(taskBytes, &task); err != nil {
		return protocol.TaskResult{}, fmt.Errorf("failed to unmarshal task: %w", err)
	}

	image := task.Image
	if image == "" {
		image = fallbackImage
	}

	fmt.Printf("👑 [Master] Triggering local Docker execution for task %s (image: %s)...\n", task.TaskID, image)

	if err := o.Engine.PullImage(ctx, image); err != nil {
		fmt.Printf("⚠️  [Master] Local image pull warning: %v\n", err)
	}

	stdoutBytes, err := o.Engine.Execute(ctx, image, taskBytes)
	if err != nil {
		return protocol.TaskResult{}, fmt.Errorf("local execution failed: %w", err)
	}

	var res protocol.TaskResult
	if err := json.Unmarshal(stdoutBytes, &res); err != nil || len(res.Output) == 0 {
		res = protocol.TaskResult{
			TaskID:    task.TaskID,
			ProjectID: o.Config.Name,
			Status:    "ok",
			Output:    json.RawMessage(stdoutBytes),
			WorkerID:  "master-local",
			IP:        "127.0.0.1",
		}
	} else {
		if res.TaskID == "" {
			res.TaskID = task.TaskID
		}
		res.ProjectID = o.Config.Name
		if res.WorkerID == "" {
			res.WorkerID = "master-local"
		}
		if res.IP == "" {
			res.IP = "127.0.0.1"
		}
		if res.Status == "" {
			res.Status = "ok"
		}
	}

	return res, nil
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

	// 2. Build & push container img
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

	// Respond to info requests
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

	// 7. Load persisted state and compute set difference
	persistedVerified, err := o.Store.GetVerifiedResults(cfg.Name)
	if err != nil {
		return fmt.Errorf("failed to load verified results from store: %w", err)
	}
	persistedRaw, err := o.Store.GetRawResults(cfg.Name)
	if err != nil {
		return fmt.Errorf("failed to load raw results from store: %w", err)
	}

	rawByTask := make(map[string][]protocol.TaskResult)
	for _, r := range persistedRaw {
		rawByTask[r.TaskID] = append(rawByTask[r.TaskID], r)
	}

	taskOutputs := make(map[string]map[string][]protocol.TaskResult)
	taskIPs := make(map[string]map[string]bool)
	verifiedResults := make(map[string]protocol.TaskResult)
	for k, v := range persistedVerified {
		verifiedResults[k] = v
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

	taskOrder := make([]string, 0, totalTasks)
	tasksList := make([]*managedTask, 0, totalTasks)
	tasksMap := make(map[string]*managedTask)

	for idx, rawTask := range rawTasks {
		var taskObj map[string]any
		if err := json.Unmarshal(rawTask, &taskObj); err != nil {
			return fmt.Errorf("invalid task JSON at index %d: %w", idx, err)
		}

		taskID, ok := taskObj["task_id"].(string)
		if !ok || taskID == "" {
			taskID = fmt.Sprintf("task-%04d", idx)
			taskObj["task_id"] = taskID
		}
		taskOrder = append(taskOrder, taskID)

		taskObj["project_id"] = cfg.Name
		taskObj["image"] = imageTag

		augmentedBytes, err := json.Marshal(taskObj)
		if err != nil {
			return fmt.Errorf("failed to marshal task %s: %w", taskID, err)
		}

		mt := &managedTask{
			taskID:       taskID,
			rawBytes:     augmentedBytes,
			assignedIPs:  make(map[string]time.Time),
			completedIPs: make(map[string]bool),
			iteration:    1,
		}
		tasksMap[taskID] = mt

		// If task already has verified consensus in DB, omit from tasksList
		if _, ok := verifiedResults[taskID]; ok {
			continue
		}

		// Rehydrate partial results if recorded in DB
		if rawList, ok := rawByTask[taskID]; ok {
			if taskIPs[taskID] == nil {
				taskIPs[taskID] = make(map[string]bool)
			}
			if taskOutputs[taskID] == nil {
				taskOutputs[taskID] = make(map[string][]protocol.TaskResult)
			}
			for _, r := range rawList {
				if r.Status != "ok" {
					continue
				}
				workerIP := getWorkerIP(r.IP, r.WorkerID)
				if !o.SendDup && taskIPs[taskID][workerIP] {
					continue
				}
				taskIPs[taskID][workerIP] = true
				mt.completedIPs[workerIP] = true
				mt.completedCount++

				normOutput := normalizeJSON(r.Output)
				taskOutputs[taskID][normOutput] = append(taskOutputs[taskID][normOutput], r)
			}

			// Check if consensus is achieved with restored results
			distinctIPCount := len(taskIPs[taskID])
			totalResultCount := 0
			for _, matching := range taskOutputs[taskID] {
				totalResultCount += len(matching)
			}
			consensusAchieved := false
			for _, matching := range taskOutputs[taskID] {
				reached := false
				if o.SendDup {
					reached = totalResultCount >= reqRedundancy && len(matching) >= threshold
				} else {
					reached = distinctIPCount >= reqRedundancy && len(matching) >= threshold
				}
				if reached {
					verifiedResults[taskID] = matching[0]
					_ = o.Store.SaveVerifiedResult(cfg.Name, matching[0])
					consensusAchieved = true
					break
				}
			}
			if consensusAchieved {
				continue
			}
		}

		// Task still needs processing: include in dispatch set
		tasksList = append(tasksList, mt)
	}

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

		redundancy := cfg.Verification.Redundancy
		if redundancy <= 0 {
			redundancy = 1
		}

		// Clear any existing in-flight assignment for this worker
		for _, mt := range tasksList {
			delete(mt.assignedIPs, workerKey)
		}

		var selected *managedTask

		isEligible := func(mt *managedTask) bool {
			iter := mt.iteration
			if iter <= 0 {
				iter = 1
			}
			effRedundancy := redundancy * iter
			if o.SendDup {
				activeCount := 0
				for key, t := range mt.assignedIPs {
					if key != workerKey && time.Since(t) < 60*time.Second {
						activeCount++
					}
				}
				return mt.completedCount+activeCount < effRedundancy
			} else {
				if mt.completedIPs[workerIP] {
					return false
				}
				activeOther := 0
				for otherIP, t := range mt.assignedIPs {
					if otherIP != workerIP && time.Since(t) < 60*time.Second && !mt.completedIPs[otherIP] {
						activeOther++
					}
				}
				return len(mt.completedIPs)+activeOther < effRedundancy
			}
		}

		// Pass 1: Prioritize unverified tasks this worker has not completed
		for _, mt := range tasksList {
			if _, verified := verifiedResults[mt.taskID]; !verified {
				if !mt.completedIPs[workerIP] && isEligible(mt) {
					selected = mt
					break
				}
			}
		}

		// Pass 2: Remaining redundant runs of verified tasks as TRAP TASKS
		if selected == nil {
			for _, mt := range tasksList {
				if _, verified := verifiedResults[mt.taskID]; verified {
					if !mt.completedIPs[workerIP] && isEligible(mt) {
						selected = mt
						break
					}
				}
			}
		}

		// Pass 3: If send-dup is enabled, allow tasks this workerIP already completed
		if selected == nil && o.SendDup {
			for _, mt := range tasksList {
				if isEligible(mt) {
					selected = mt
					break
				}
			}
		}

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

	// 8. Collect Results & Consensus Verification
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

	// 7. Pipe Verified Results to Reducer
	fmt.Printf("📊 Step 6/6: Piping verified batch to reducer (%s %s)...\n", cfg.Reducer.Runtime, cfg.Reducer.Source)
	reducerOutput, err := o.runReducer(ctx, cfg, taskOrder, verifiedResults)
	if err != nil {
		return fmt.Errorf("reducer script failed: %w", err)
	}

	fmt.Println("\n🎉 Computation Complete! Reducer Output:")
	fmt.Println(string(reducerOutput))

	return nil
}

func (o *Orchestrator) runGenerator(ctx context.Context, cfg *protocol.ProjectConfig) ([]json.RawMessage, error) {
	scriptPath := cfg.Generator.Source
	if !filepath.IsAbs(scriptPath) {
		scriptPath = filepath.Join(o.ProjectDir, scriptPath)
	}

	runtimeBin := cfg.Generator.Runtime
	if runtimeBin == "" {
		runtimeBin = "python3"
	}

	cmd := exec.CommandContext(ctx, runtimeBin, scriptPath)
	cmd.Dir = o.ProjectDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("generator execution failed: %w\nstderr: %s", err, stderr.String())
	}
	var tasks []json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &tasks); err != nil {
		return nil, fmt.Errorf("generator output is not a valid JSON array: %w\noutput: %s", err, stdout.String())
	}
	// fmt.Print(tasks[0])
	return tasks, nil
}

func (o *Orchestrator) collectAndVerifyResults(
	ctx context.Context,
	cfg *protocol.ProjectConfig,
	taskOrder []string,
	taskMu *sync.Mutex,
	tasksMap map[string]*managedTask,
	isDone *bool,
	verifiedResults map[string]protocol.TaskResult,
	taskOutputs map[string]map[string][]protocol.TaskResult,
	taskIPs map[string]map[string]bool,
) (map[string]protocol.TaskResult, error) {
	totalDistinct := len(taskOrder)
	if len(verifiedResults) >= totalDistinct && tasksMap == nil {
		return verifiedResults, nil
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
	maxIterations := cfg.Verification.Iterations
	if maxIterations <= 0 {
		maxIterations = 1
	}

	var mtx sync.Mutex
	doneCh := make(chan struct{})
	var doneOnce sync.Once

	var (
		graceTimer   *time.Timer
		graceTimerMu sync.Mutex
	)

	checkTaskConsensusOrEscalation := func(mt *managedTask) {
		if _, verified := verifiedResults[mt.taskID]; verified {
			return
		}
		k := mt.iteration
		if k <= 0 {
			k = 1
			mt.iteration = 1
		}
		effThreshold := threshold * k
		effRedundancy := reqRedundancy * k

		var consensusNorm string
		var consensusRes protocol.TaskResult
		foundConsensus := false
		for norm, matches := range taskOutputs[mt.taskID] {
			if len(matches) >= effThreshold {
				consensusNorm = norm
				consensusRes = matches[0]
				foundConsensus = true
				break
			}
		}

		if foundConsensus {
			verifiedResults[mt.taskID] = consensusRes
			if o.Store != nil {
				_ = o.Store.SaveVerifiedResult(cfg.Name, consensusRes)
			}
			matchCount := len(taskOutputs[mt.taskID][consensusNorm])
			totalCount := 0
			for _, m := range taskOutputs[mt.taskID] {
				totalCount += len(m)
			}
			distinctIPCount := len(taskIPs[mt.taskID])
			if o.SendDup {
				fmt.Printf("   ✅ Task %s verified! (Consensus: %d/%d matches from %d results in iteration %d | Total Verified: %d/%d)\n",
					mt.taskID, matchCount, effThreshold, totalCount, k, len(verifiedResults), totalDistinct)
			} else {
				fmt.Printf("   ✅ Task %s verified! (Consensus: %d/%d matches from %d distinct IPs in iteration %d | Total Verified: %d/%d)\n",
					mt.taskID, matchCount, effThreshold, distinctIPCount, k, len(verifiedResults), totalDistinct)
			}

			// Blacklist any worker that submitted an output differing from the consensus (minority result)
			for otherNorm, otherList := range taskOutputs[mt.taskID] {
				if otherNorm != consensusNorm {
					for _, badRes := range otherList {
						badIP := getWorkerIP(badRes.IP, badRes.WorkerID)
						reason := fmt.Sprintf("Minority result on task %s: got %s, consensus was %s",
							mt.taskID, string(badRes.Output), string(consensusRes.Output))
						o.blacklistIP(badIP, reason, tasksMap)
					}
				}
			}
			return
		}

		// If no consensus reached, check if current iteration is exhausted
		taskMu.Lock()
		inFlightForTask := len(mt.assignedIPs) > 0
		completed := mt.completedCount
		taskMu.Unlock()

		if completed >= effRedundancy && !inFlightForTask {
			if k < maxIterations {
				mt.iteration++
				fmt.Printf("🔄 [Master] Task %s reached redundancy limit (%d) in iteration %d without consensus. Escalating to iteration %d/%d (Redundancy: %d, Threshold: %d)\n",
					mt.taskID, effRedundancy, k, mt.iteration, maxIterations, reqRedundancy*mt.iteration, threshold*mt.iteration)
			} else {
				// Reached max iterations without consensus -> Fallback to local master Docker execution!
				fmt.Printf("⚠️  [Master] Task %s reached max iterations (%d) without consensus. Falling back to local master Docker execution...\n",
					mt.taskID, maxIterations)
				localRes, err := o.executeTaskLocally(ctx, mt.rawBytes, cfg.Container.ImageTag)
				if err == nil {
					verifiedResults[mt.taskID] = localRes
					if o.Store != nil {
						_ = o.Store.SaveVerifiedResult(cfg.Name, localRes)
					}
					fmt.Printf("👑 [Master] Local execution established ground truth for task %s: %s\n",
						mt.taskID, string(localRes.Output))
					canonical := normalizeJSON(localRes.Output)
					for otherNorm, otherList := range taskOutputs[mt.taskID] {
						if otherNorm != canonical {
							for _, badRes := range otherList {
								badIP := getWorkerIP(badRes.IP, badRes.WorkerID)
								reason := fmt.Sprintf("Diverged from master local ground truth on task %s: got %s, ground truth was %s",
									mt.taskID, string(badRes.Output), string(localRes.Output))
								o.blacklistIP(badIP, reason, tasksMap)
							}
						}
					}
				} else {
					fmt.Printf("❌ [Master] Local execution failed for task %s: %v\n", mt.taskID, err)
				}
			}
		}
	}

	var checkCompletion func()

	startGraceTimer := func() {
		graceTimerMu.Lock()
		defer graceTimerMu.Unlock()
		if graceTimer != nil {
			graceTimer.Stop()
		}
		graceTimer = time.AfterFunc(1500*time.Millisecond, func() {
			mtx.Lock()
			defer mtx.Unlock()

			if checkCompletion != nil {
				checkCompletion()
			}

			if len(verifiedResults) < totalDistinct {
				return
			}
			if taskMu != nil {
				taskMu.Lock()
				hasFlight := false
				for _, mt := range tasksMap {
					if len(mt.assignedIPs) > 0 {
						hasFlight = true
						break
					}
				}
				if !hasFlight {
					if isDone != nil {
						*isDone = true
					}
					doneOnce.Do(func() {
						close(doneCh)
					})
				}
				taskMu.Unlock()
			} else {
				if isDone != nil {
					*isDone = true
				}
				doneOnce.Do(func() {
					close(doneCh)
				})
			}
		})
	}

	defer func() {
		graceTimerMu.Lock()
		if graceTimer != nil {
			graceTimer.Stop()
		}
		graceTimerMu.Unlock()
	}()

	checkCompletion = func() {
		if tasksMap != nil {
			for _, mt := range tasksMap {
				if _, verified := verifiedResults[mt.taskID]; !verified {
					checkTaskConsensusOrEscalation(mt)
				}
			}
		}

		if len(verifiedResults) < totalDistinct {
			return
		}
		if taskMu == nil || tasksMap == nil {
			if isDone != nil {
				*isDone = true
			}
			doneOnce.Do(func() {
				close(doneCh)
			})
			return
		}

		taskMu.Lock()
		allMet := true
		inFlight := false
		for _, mt := range tasksMap {
			if len(mt.assignedIPs) > 0 {
				inFlight = true
			}
			iter := mt.iteration
			if iter <= 0 {
				iter = 1
			}
			if mt.completedCount < reqRedundancy*iter {
				allMet = false
			}
		}
		taskMu.Unlock()

		if allMet {
			graceTimerMu.Lock()
			if graceTimer != nil {
				graceTimer.Stop()
			}
			graceTimerMu.Unlock()

			taskMu.Lock()
			if isDone != nil {
				*isDone = true
			}
			taskMu.Unlock()
			doneOnce.Do(func() {
				close(doneCh)
			})
		} else if !inFlight {
			startGraceTimer()
		}
	}

	checkCompletion()

	subj := transport.ResultsSubject(cfg.Name)
	sub, err := o.nc.Subscribe(subj, func(msg *nats.Msg) {
		var res protocol.TaskResult
		if err := json.Unmarshal(msg.Data, &res); err != nil {
			fmt.Printf("⚠️  [Master] Received invalid result payload on %s: %v\n", subj, err)
			return
		}

		if res.TaskID == "" {
			return
		}

		workerIP := getWorkerIP(res.IP, res.WorkerID)
		workerKey := workerIP
		if o.SendDup && res.WorkerID != "" {
			workerKey = res.WorkerID
		}

		// Discard result if worker is blacklisted
		if o.isIPBlacklisted(workerIP) {
			fmt.Printf("⛔ [Master] Discarding result for task %s from blacklisted IP %s (Worker: %s)\n", res.TaskID, workerIP, res.WorkerID)
			return
		}

		if taskMu != nil {
			taskMu.Lock()
			if mt, ok := tasksMap[res.TaskID]; ok {
				if _, assigned := mt.assignedIPs[workerKey]; assigned {
					delete(mt.assignedIPs, workerKey)
					if res.Status == "ok" {
						mt.completedCount++
					}
				} else if res.Status != "ok" && mt.completedCount > 0 {
					mt.completedCount--
				}
				if res.Status == "ok" {
					mt.completedIPs[workerIP] = true
				}
			}
			taskMu.Unlock()
		}

		mtx.Lock()
		defer mtx.Unlock()

		if verified, alreadyVerified := verifiedResults[res.TaskID]; alreadyVerified {
			// This is a TRAP TASK result (or late redundant result after consensus)
			normOutput := normalizeJSON(res.Output)
			expectedOutput := normalizeJSON(verified.Output)

			if res.Status != "ok" || normOutput != expectedOutput {
				reason := fmt.Sprintf("Trap task %s output mismatch: expected %s, got %s (status: %s)",
					res.TaskID, expectedOutput, normOutput, res.Status)
				o.blacklistIP(workerIP, reason, tasksMap)
			} else {
				fmt.Printf("   🪤 Task %s trap PASSED by worker %s (IP: %s)\n", res.TaskID, res.WorkerID, workerIP)
				if o.Store != nil {
					_ = o.Store.SaveRawResult(cfg.Name, res)
				}
			}
			checkCompletion()
			return
		}

		if res.Status != "ok" {
			fmt.Printf("⚠️  [Master] Task %s returned status '%s': %s\n", res.TaskID, res.Status, res.Error)
			return
		}

		if taskIPs[res.TaskID] == nil {
			taskIPs[res.TaskID] = make(map[string]bool)
		}
		if !o.SendDup && taskIPs[res.TaskID][workerIP] {
			fmt.Printf("⚠️  [Master] Duplicate workload result for task %s from same IP %s ignored\n", res.TaskID, workerIP)
			return
		}
		taskIPs[res.TaskID][workerIP] = true

		if o.Store != nil {
			if err := o.Store.SaveRawResult(cfg.Name, res); err != nil {
				fmt.Printf("⚠️  [Master] Failed to persist raw result for task %s: %v\n", res.TaskID, err)
			}
		}

		normOutput := normalizeJSON(res.Output)
		if taskOutputs[res.TaskID] == nil {
			taskOutputs[res.TaskID] = make(map[string][]protocol.TaskResult)
		}
		taskOutputs[res.TaskID][normOutput] = append(taskOutputs[res.TaskID][normOutput], res)

		if mt, ok := tasksMap[res.TaskID]; ok {
			checkTaskConsensusOrEscalation(mt)
		}

		checkCompletion()
	})

	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to %s: %w", subj, err)
	}
	defer sub.Unsubscribe()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-doneCh:
		return verifiedResults, nil
	}
}

func (o *Orchestrator) runReducer(
	ctx context.Context,
	cfg *protocol.ProjectConfig,
	taskOrder []string,
	verifiedResults map[string]protocol.TaskResult,
) ([]byte, error) {
	scriptPath := cfg.Reducer.Source
	if !filepath.IsAbs(scriptPath) {
		scriptPath = filepath.Join(o.ProjectDir, scriptPath)
	}

	runtimeBin := cfg.Reducer.Runtime
	if runtimeBin == "" {
		runtimeBin = "python3"
	}

	orderedList := make([]protocol.TaskResult, 0, len(taskOrder))
	for _, taskID := range taskOrder {
		if res, ok := verifiedResults[taskID]; ok {
			orderedList = append(orderedList, res)
		}
	}

	batch := protocol.ReducerBatch{
		ProjectID: cfg.Name,
		Results:   orderedList,
	}
	batchBytes, err := json.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal reducer batch: %w", err)
	}

	cmd := exec.CommandContext(ctx, runtimeBin, scriptPath)
	cmd.Dir = o.ProjectDir
	cmd.Stdin = bytes.NewReader(batchBytes)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("reducer execution failed: %w\nstderr: %s\nstdout: %s",
			err, stderr.String(), stdout.String())
	}

	return stdout.Bytes(), nil
}

func normalizeJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	canonical, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	return string(canonical)
}

func getWorkerIP(resIP, workerID string) string {
	if resIP != "" {
		return resIP
	}
	parts := strings.Split(workerID, "-")
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return workerID
}

