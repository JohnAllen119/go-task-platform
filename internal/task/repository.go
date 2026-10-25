package task

import (
	"context"
	"database/sql"
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
