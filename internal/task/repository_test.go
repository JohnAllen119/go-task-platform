package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	_ "github.com/go-sql-driver/mysql"
	"os"
	"sync"
	"testing"
	"time"
)

type startResult struct {
	started bool
	err     error
}

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
func TestCreateTaskSuccess(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	payload := json.RawMessage(`{"to":"test@example.com"}`)
	id, err := CreateTask(ctx, db, "email", payload)
	if err != nil {
		t.Fatalf("create task failed:%v", err)
	}
	t.Cleanup(func() {
		_, err := db.ExecContext(ctx, `
		DELETE FROM tasks
		WHERE id=?
		`, id)
		if err != nil {
			t.Fatalf("cleanup failed:%v", err)
		}
	})
	if id <= 0 {
		t.Fatalf("id<=0:%d", id)
	}
	task, err := FindByID(ctx, db, id)
	if err != nil {
		t.Fatalf("findbyid failed:%v", err)
	}
	if task.TaskType != "email" || task.Status != "pending" {
		t.Fatalf("expect tasktype=email,get:%s,expect status=pending,get:%s", task.TaskType, task.Status)
	}
}

func TestCreateTaskInvalidPayload(t *testing.T) {
	db := openTestDB(t)
	payload := json.RawMessage(`{"to":`)
	ctx := context.Background()
	id, err := CreateTask(ctx, db, "email", payload)
	if id != 0 {
		t.Cleanup(func() {
			_, err := db.ExecContext(ctx, `
			DELETE FROM tasks
			WHERE id=?
			`, id)
			if err != nil {
				t.Fatalf("delete tasks failed,:%v", err)
			}
		})
	}
	if err == nil {
		t.Fatalf("expected error for invalid JSON, got nil")
	}
	if id != 0 {

		t.Fatalf("create invalid task got id:%d", id)
	}

}
func TestTryStartTaskSuccess(t *testing.T) {
	db := openTestDB(t)
	taskType := "email"
	payload := `{"to":"test@exmaple.com"}`

	result, err := db.Exec(`
	INSERT INTO tasks(task_type,payload)
	VALUES(?,?)
	`, taskType, payload)
	if err != nil {
		t.Fatalf("insert failed:%v", err)
	}
	lid, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get id failed:%v", err)
	}
	t.Cleanup(func() {
		_, err := db.Exec(`
		DELETE FROM tasks
		WHERE id=?
		`, lid)
		if err != nil {
			t.Fatalf("cleanup failed:%v", err)
		}
	})
	ctx := context.Background()
	est, err := TryStartTask(ctx, db, uint64(lid))

	if err != nil {
		t.Fatalf("trystartedtask failed")
	}
	if est != true {
		t.Fatalf("start failed")
	}
	var sta string
	var started bool
	err = db.QueryRow(`
	SELECT status,started_at IS NOT NULL
	FROM tasks
	WHERE id=?
	`, lid).Scan(&sta, &started)
	if err != nil {
		t.Fatalf("select failed:%v", err)
	}
	if sta != "running" {
		t.Fatalf("expect:running,get:%s", sta)
	}
	if !started {
		t.Fatal("started_at is NULL")
	}

}
func TestTryStartTaskAlreadyRunning(t *testing.T) {
	db := openTestDB(t)
	taskType := "email"
	payload := `{"to":"test@example.com"}`
	result, err := db.Exec(`
	INSERT INTO tasks(task_type,payload)
	VALUES(?,?)
	`, taskType, payload)
	if err != nil {
		t.Fatalf("insert failed:%v", err)
	}
	ra, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get lastinsertid failed:%v", err)
	}
	if ra <= 0 {
		t.Fatalf("ra<=0")
	}
	t.Cleanup(func() {
		_, err := db.Exec(`
		DELETE FROM tasks
		WHERE id=?
		`, ra)
		if err != nil {
			t.Fatalf("clean up failed:%v", err)
		}
	})
	ctx := context.Background()
	tst, err := TryStartTask(ctx, db, uint64(ra))
	if err != nil {
		t.Fatalf("trystarttask failed:%v", err)
	}
	if !tst {
		t.Fatalf("start task filed")
	}
	tst, err = TryStartTask(ctx, db, uint64(ra))
	if err != nil {
		t.Fatalf("trystarttask failed:%v", err)
	}
	if tst {
		t.Fatalf("start task shouldn't succeed")
	}
	var sta string
	err = db.QueryRow(`
	SELECT status
	FROM tasks
	WHERE id=?
	`, ra).Scan(&sta)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if sta != "running" {
		t.Fatalf("expect sta=running,get:%s", sta)
	}

}
func TestTryStartTaskConcurrent(t *testing.T) {
	db := openTestDB(t)
	taskType := "email"
	payload := `{"to":"test@example.com"}`
	ctx := context.Background()
	result, err := db.Exec(`
	INSERT INTO tasks(task_type,payload)
	VALUES(?,?)
	`, taskType, payload)
	if err != nil {
		t.Fatalf("insert failed:%v", err)
	}
	ra, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get lastinsertid failed:%v", err)
	}
	if ra <= 0 {
		t.Fatalf("ra<=0")
	}
	t.Cleanup(func() {
		_, err := db.Exec(`
		DELETE FROM tasks
		WHERE id=?
		`, ra)
		if err != nil {
			t.Fatalf("clean up failed:%v", err)
		}
	})
	success := 0
	failed := 0
	var wg sync.WaitGroup
	wg.Add(10)

	ch := make(chan startResult, 10)

	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()

			est, err := TryStartTask(ctx, db, uint64(ra))
			ch <- startResult{started: est, err: err}
		}()
	}

	wg.Wait()
	close(ch)

	for v := range ch {
		if v.err != nil {
			t.Fatalf("TryStartTask failed: %v", v.err)
		}

		if v.started {
			success++
		} else {
			failed++
		}
	}
	if success != 1 && failed != 9 {
		t.Fatalf("success!=1,failed!=9")
	}
	var sta string
	err = db.QueryRow(`
	SELECT status
	FROM tasks
	WHERE id=?
	`, ra).Scan(&sta)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if sta != "running" {
		t.Fatalf("expect sta:running,get:%s", sta)
	}

}
