package worker

import (
	"context"
	"database/sql"
	"github.com/JohnAllen119/go-task-platform/internal/task"
	"sync"
)

type Result struct {
	TaskID  uint64
	Started bool
	Err     error
}

func runWorker(
	ctx context.Context,
	db *sql.DB,
	ids <-chan uint64,
	results chan<- Result,
) {
	for id := range ids {
		var result Result
		res, err := task.TryStartTask(ctx, db, id)
		result.TaskID = id
		result.Started = res
		result.Err = err
		results <- result
	}
}
func RunPool(
	ctx context.Context,
	db *sql.DB,
	ids <-chan uint64,
	workerCount int,
) <-chan Result {
	var wg sync.WaitGroup
	wg.Add(workerCount)
	results := make(chan Result)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer wg.Done()
			runWorker(ctx, db, ids, results)
		}()

	}
	go func() {
		wg.Wait()
		close(results)
	}()
	return results
}
