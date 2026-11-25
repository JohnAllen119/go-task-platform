package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/JohnAllen119/go-task-platform/internal/task"
	"sync"
	"time"
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
	for {
		select {
		case <-ctx.Done():
			return
		case id, ok := <-ids:
			if !ok {
				return
			}
			var result Result
			token, res, err := task.TryStartTask(ctx, db, id)

			if err == nil && res {

				t, founderr := task.FindByID(ctx, db, id)
				if founderr != nil {
					err = founderr
				} else {
					execCtx, cancelExec := context.WithCancel(ctx)
					heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
					heartbeatDone := make(chan error, 1)
					go func() {
						heartbeatDone <- runHeartbeat(
							heartbeatCtx,
							db,
							id,
							token,
							10*time.Second,
							cancelExec,
							task.RenewLease,
						)
					}()
					execErr := Execute(execCtx, t.TaskType, t.Payload)
					stopHeartbeat()
					heartbeatErr := <-heartbeatDone
					cancelExec()
					if heartbeatErr != nil {
						err = errors.Join(execErr, heartbeatErr)
					} else if execErr != nil {
						err = execErr
						updated, markErr := task.MarkFailed(ctx, db, id, execErr.Error(), token)
						if markErr != nil {
							err = errors.Join(execErr, markErr)
						} else if !updated {
							err = errors.Join(
								execErr,
								fmt.Errorf("task %d: status update failed", id),
							)
						}

					} else {
						updated, markErr := task.MarkSucceeded(ctx, db, id, token)

						if markErr != nil {
							err = fmt.Errorf("mark succeeded failed: %w", markErr)
						} else if !updated {
							err = fmt.Errorf("task %d: status update failed", id)
						}

					}
				}

			}

			result = Result{
				TaskID:  id,
				Started: res,
				Err:     err,
			}
			select {
			case <-ctx.Done():
				return
			case results <- result:
			}

		}
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
