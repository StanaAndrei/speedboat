package orchestrator

import (
	"time"

	"speedboat/internal/protocol"
)

// selectTaskForWorker selects the next task to dispatch to a requesting worker.
// Selection priority:
// 1. Unverified tasks not yet completed by this worker, within redundancy limit
// 2. Already verified tasks dispatched as TRAP tasks (spot checks)
// 3. If sendDup is enabled, duplicate assignments to workers that already completed the task
func selectTaskForWorker(
	tasksList []*managedTask,
	verifiedResults map[string]protocol.TaskResult,
	workerIP string,
	workerKey string,
	sendDup bool,
	redundancy int,
) *managedTask {
	if redundancy <= 0 {
		redundancy = 1
	}

	isEligible := func(mt *managedTask) bool {
		iter := mt.iteration
		if iter <= 0 {
			iter = 1
		}
		effRedundancy := redundancy * iter
		if sendDup {
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
				return mt
			}
		}
	}

	// Pass 2: Remaining redundant runs of verified tasks as TRAP TASKS
	for _, mt := range tasksList {
		if _, verified := verifiedResults[mt.taskID]; verified {
			if !mt.completedIPs[workerIP] && isEligible(mt) {
				return mt
			}
		}
	}

	// Pass 3: If send-dup is enabled, allow tasks this workerIP already completed
	if sendDup {
		for _, mt := range tasksList {
			if isEligible(mt) {
				return mt
			}
		}
	}

	return nil
}
