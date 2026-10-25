package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	_ "github.com/go-sql-driver/mysql"
	"os"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("GO_TASK_PLATFORM_TEST_DSN")
	if dsn == "" {
		t.Fatalf("get dsn failed")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("mysql open failed:%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	t.Cleanup(func() {
		db.Close()
	})
	err = db.PingContext(ctx)
	if err != nil {
		t.Fatalf("mysql ping failed:%v", err)
	}

	var dbName string
	err = db.QueryRowContext(ctx, `
	SELECT DATABASE()
	`).Scan(&dbName)
	if err != nil {
		t.Fatalf("query dbName failed:%v", err)
	}
	if dbName != "go_task_platform_test" {
		t.Fatalf("expect:go_task_platform_test,get:%s", dbName)
	}
	return db
}
func TestDatabaseConnection(t *testing.T) {
	openTestDB(t)
}
func TestFindByIDSuccess(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	result, err := db.ExecContext(ctx, `
	INSERT INTO tasks(task_type,payload,status)
	VALUES(?,?,?)

	`, "email", `{"to":"test@example.com"}`, "pending")
	if err != nil {
		t.Fatalf("exec test task failed:%v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get lastinsertid failed:%v", err)
	}
	t.Cleanup(func() {
		_, err := db.ExecContext(ctx, `
		DELETE FROM tasks
		WHERE id=?
		`, id)
		if err != nil {
			t.Fatalf("delete test task failed:%v", err)
		}
	})
	task, err := FindByID(ctx, db, uint64(id))
	if err != nil {
		t.Fatalf("findbyid failed:%v", err)
	}
	if task.ID != uint64(id) {
		t.Fatalf("expect id:%d,get:%d", uint64(id), task.ID)
	}
	if task.TaskType != "email" {
		t.Fatalf("expect tasktype:email,get:%s", task.TaskType)
	}
	if task.Status != "pending" {
		t.Fatalf("expect status:pending,get:%s", task.Status)
	}
	if task.AttemptCount != 0 {
		t.Fatalf("expect attemptcount=0,get:%d", task.AttemptCount)
	}
	if task.MaxAttempts != 3 {
		t.Fatalf("expect maxattempts=3,get:%v", task.MaxAttempts)
	}
	if task.LastError != nil {
		t.Fatalf("expect lasterror=nil,get:%v", task.LastError)
	}
	if task.StartedAt != nil {
		t.Fatalf("expect startedat=nil,get:%v", task.StartedAt)
	}
	if task.FinishedAt != nil {
		t.Fatalf("expect finishedat=nil,get:%v", task.FinishedAt)
	}
	if task.CreatedAt.IsZero() {
		t.Fatalf("expect createdat!=nil")
	}
	if task.UpdatedAt.IsZero() {
		t.Fatalf("expect updateat!=nil")
	}
	var payload struct {
		To string `json:"to"`
	}
	err = json.Unmarshal(task.Payload, &payload)
	if err != nil {
		t.Fatalf("unmarshal payload failed:%v", err)
	}
	if payload.To != "test@example.com" {
		t.Fatalf("expect:test@example.com,get:%s", payload.To)
	}
}
func TestFindByIDNotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	var id uint64
	id = 20
	var task Task
	err := db.QueryRowContext(ctx, `
	SELECT id
	FROM tasks
	WHERE id=?
	`, id).Scan(&task.ID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("query task failed")
	}
	task, err = FindByID(ctx, db, id)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expect err=sql.ErrNoRows,get:%v", err)
	}

}
