package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"speedboat/internal/protocol"
	"speedboat/internal/runtime"
	"speedboat/internal/transport"

	"github.com/nats-io/nats.go"
)

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
		for _, mt := range tasksMap {
			if _, verified := verifiedResults[mt.taskID]; !verified {
				checkTaskConsensusOrEscalation(mt)
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
		workerKey := res.WorkerID
		if workerKey == "" {
			workerKey = workerIP
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
