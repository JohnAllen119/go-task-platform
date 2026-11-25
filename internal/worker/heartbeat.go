package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func runHeartbeat(
	ctx context.Context,
	db *sql.DB,
	id uint64,
	token string,
	interval time.Duration,
	cancelExec context.CancelFunc,
	renewFunc func(context.Context, *sql.DB, uint64, string) (bool, error),
) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			renew, err := renewFunc(ctx, db, id, token)
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
