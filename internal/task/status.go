package task

import (
	"context"
	"database/sql"
	"fmt"
)

func MarkSucceeded(
	ctx context.Context,
	db *sql.DB,
	id uint64,
	leaseToken string,
) (bool, error) {
	result, err := db.ExecContext(ctx, `
	UPDATE tasks
	SET status=?,finished_at=NOW()
	WHERE id=? AND status='running' AND lease_token=? AND lease_expires_at>NOW(6)
	`, "succeeded", id, leaseToken)
	if err != nil {
		return false, err
	}
	af, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if af > 1 {
		return false, fmt.Errorf("expect rows affect=1,got:%d", af)
	}
	if af == 0 {
		return false, nil
	}
	return true, nil
}
func MarkFailed(
	ctx context.Context,
	db *sql.DB,
	id uint64,
	reason string,
	leaseToken string,
) (bool, error) {
	result, err := db.ExecContext(ctx, `
	UPDATE tasks
	SET status=?,last_error=?,finished_at=NOW()
	WHERE id=? AND status='running' AND lease_token=? AND lease_expires_at>NOW(6)
	`, "failed", reason, id, leaseToken)
	if err != nil {
		return false, err
	}
	ra, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if ra > 1 {
		return false, fmt.Errorf("expect rows affect=1,got:%d", ra)
	}
	if ra == 0 {
		return false, nil
	}
	return true, nil
}
