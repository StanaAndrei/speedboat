package orchestrator

import (
	"math/rand/v2"
	"testing"
)

func TestTaskShuffling(t *testing.T) {
	total := 100
	tasksList := make([]*managedTask, total)
	for i := 0; i < total; i++ {
		tasksList[i] = &managedTask{
			taskID: string(rune('A' + (i % 26))),
		}
	}

	orig := make([]*managedTask, total)
	copy(orig, tasksList)

	rand.Shuffle(len(tasksList), func(i, j int) {
		tasksList[i], tasksList[j] = tasksList[j], tasksList[i]
	})

	sameOrderCount := 0
	for i := 0; i < total; i++ {
		if tasksList[i] == orig[i] {
			sameOrderCount++
		}
	}

	if sameOrderCount == total {
		t.Fatalf("expected tasksList to be shuffled in random order, but order was identical")
	}
}
