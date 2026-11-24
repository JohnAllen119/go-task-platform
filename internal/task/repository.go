package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
)

func FindByID(ctx context.Context, db *sql.DB, id uint64) (Task, error) {
	var task Task
	err := db.QueryRowContext(ctx, `
	SELECT id,task_type,payload,status,attempt_count,max_attempts,last_error,created_at,updated_at,started_at,finished_at
	FROM tasks
	WHERE id=?
	`, id).Scan(&task.ID,
		&task.TaskType,
		&task.Payload,
		&task.Status,
		&task.AttemptCount,
		&task.MaxAttempts,
		&task.LastError,
		&task.CreatedAt,
		&task.UpdatedAt,
		&task.StartedAt,
		&task.FinishedAt)
	if err != nil {
		return Task{}, err
	}
	return task, nil
}
func CreateTask(
	ctx context.Context,
	db *sql.DB,
	taskType string,
	payload json.RawMessage,
) (uint64, error) {
	result, err := db.ExecContext(ctx, `
	INSERT INTO tasks(task_type,payload)
	VALUES(?,?)
	`, taskType, payload)
	if err != nil {
		return 0, err
	}

	newID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if newID <= 0 {
		return 0, fmt.Errorf("newID<=0,get:%d", newID)
	}
	return uint64(newID), nil
}
func TryStartTask(ctx context.Context, db *sql.DB, id uint64) (string, bool, error) {
	s, err := generateLeaseToken()
	if err != nil {
		return "", false, err
	}
	result, err := db.ExecContext(ctx, `
	UPDATE tasks
	SET status='running',started_at=CURRENT_TIMESTAMP,lease_expires_at = NOW(6) + INTERVAL 30 SECOND,lease_token=?
	WHERE id=? AND (status='pending' OR (status='running' AND lease_expires_at<=NOW(6)))
	`, s, id)
	if err != nil {
		return "", false, err
	}
	ra, err := result.RowsAffected()
	if err != nil {
		return "", false, err
	}
	if ra > 1 {
		return "", false, fmt.Errorf("ra>1")
	}
	if ra == 0 {
		return "", false, nil
	}
	return s, true, nil
}
func RenewLease(
	ctx context.Context,
	db *sql.DB,
	id uint64,
	leaseToken string,
) (bool, error) {
	result, err := db.ExecContext(ctx, `
	UPDATE tasks
	SET lease_expires_at=NOW(6)+INTERVAL 30 SECOND
	WHERE id=? AND status=? AND lease_token=? AND lease_expires_at>NOW(6)
	`, id, "running", leaseToken)
	if err != nil {
		return false, err
	}
	ra, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if ra == 0 {
		return false, nil
	}
	if ra > 1 {
		return false, fmt.Errorf("ra>1")
	}
	return true, nil
}
