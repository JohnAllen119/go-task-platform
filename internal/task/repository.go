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
