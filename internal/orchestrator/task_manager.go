package orchestrator

import (
	"encoding/json"
	"fmt"
	"time"

	"speedboat/internal/protocol"
)

type managedTask struct {
	taskID         string
	rawBytes       []byte
	assignedIPs    map[string]time.Time
	completedIPs   map[string]bool
	completedCount int
	iteration      int
}

type TaskState struct {
	TasksList       []*managedTask
	TasksMap        map[string]*managedTask
	TaskOrder       []string
	VerifiedResults map[string]protocol.TaskResult
	TaskOutputs     map[string]map[string][]protocol.TaskResult
	TaskIPs         map[string]map[string]bool
}

// reconcileTasksFromStore initializes the task pool from raw generated tasks and reconciles
// it with historical verified and raw results loaded from the persistence store.
func reconcileTasksFromStore(
	rawTasks []json.RawMessage,
	projectName string,
	imageTag string,
	reqRedundancy int,
	threshold int,
	sendDup bool,
	persistedVerified map[string]protocol.TaskResult,
	persistedRaw []protocol.TaskResult,
	saveVerifiedFn func(projectID string, res protocol.TaskResult) error,
) (*TaskState, error) {
	totalTasks := len(rawTasks)
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

	taskOrder := make([]string, 0, totalTasks)
	tasksList := make([]*managedTask, 0, totalTasks)
	tasksMap := make(map[string]*managedTask)

	for idx, rawTask := range rawTasks {
		var taskObj map[string]any
		if err := json.Unmarshal(rawTask, &taskObj); err != nil {
			return nil, fmt.Errorf("invalid task JSON at index %d: %w", idx, err)
		}

		taskID, ok := taskObj["task_id"].(string)
		if !ok || taskID == "" {
			taskID = fmt.Sprintf("task-%04d", idx)
			taskObj["task_id"] = taskID
		}
		taskOrder = append(taskOrder, taskID)

		taskObj["project_id"] = projectName
		taskObj["image"] = imageTag

		augmentedBytes, err := json.Marshal(taskObj)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal task %s: %w", taskID, err)
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
				if !sendDup && taskIPs[taskID][workerIP] {
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
				if sendDup {
					reached = totalResultCount >= reqRedundancy && len(matching) >= threshold
				} else {
					reached = distinctIPCount >= reqRedundancy && len(matching) >= threshold
				}
				if reached {
					verifiedResults[taskID] = matching[0]
					if saveVerifiedFn != nil {
						_ = saveVerifiedFn(projectName, matching[0])
					}
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

	return &TaskState{
		TasksList:       tasksList,
		TasksMap:        tasksMap,
		TaskOrder:       taskOrder,
		VerifiedResults: verifiedResults,
		TaskOutputs:     taskOutputs,
		TaskIPs:         taskIPs,
	}, nil
}
