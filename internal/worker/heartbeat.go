package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/JohnAllen119/go-task-platform/internal/task"
	"time"
)

func runHeartbeat(
	ctx context.Context,
	db *sql.DB,
	id uint64,
	token string,
	interval time.Duration,
	cancelExec context.CancelFunc,
) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			renew, err := task.RenewLease(ctx, db, id, token)
			if err != nil {
				if errors.Is(err, context.Canceled) && ctx.Err() != nil {
					return nil
				}
				cancelExec()
				return err
			}
			if !renew {
				cancelExec()
				return fmt.Errorf("lease renewal rejected for task %d", id)
			}
		case <-ctx.Done():
			return nil
		}
	}
}
