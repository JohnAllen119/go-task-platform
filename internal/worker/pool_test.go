package worker

import (
	"context"
	"database/sql"

	"os"
	"testing"
	"time"
)

func openWorkerTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("GO_TASK_PLATFORM_TEST_DSN")
	if dsn == "" {
		t.Fatalf("get dsn failed")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("mysql open failed:%v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})
	ctx := context.Background()
	err = db.PingContext(ctx)
	if err != nil {
		t.Fatalf("mysql connect failed:%v", err)
	}
	var dbName string
	err = db.QueryRowContext(ctx, `
	SELECT DATABASE()
	`).Scan(&dbName)
	if err != nil {
		t.Fatalf("db query failed:%v", err)
	}
	if dbName != "go_task_platform_test" {
		t.Fatalf("expect:go_task_platform_test,get:%s", dbName)
	}
	return db
}
func TestRunPoolMultipleTasks(t *testing.T) {
	db := openWorkerTestDB(t)
	ids := make(chan uint64, 5)
	var insertedIDs []uint64
	t.Cleanup(func() {
		for _, id := range insertedIDs {
			_, err := db.Exec(`
			DELETE FROM tasks
			WHERE id=?
			`, id)
			if err != nil {
				t.Errorf("%v", err)
			}
		}
	})
	for i := 0; i < 5; i++ {
		result, err := db.Exec(`
		INSERT INTO tasks(task_type,payload)
		VALUES(?,?)
		`, "email", `{"to":"test@example.com"}`)
		if err != nil {
			t.Fatalf("insert failed")
		}
		id, err := result.LastInsertId()

		if err != nil {
			t.Fatalf("get id failed:%v", err)
		}
		if id <= 0 {
			t.Fatalf("id<=0")
		}
		taskID := uint64(id)
		ids <- taskID
		insertedIDs = append(insertedIDs, taskID)
	}

	close(ids)
	results := RunPool(context.Background(), db, ids, 3)
	count := 0
	u := make(map[uint64]bool)
	expected := make(map[uint64]bool)
	for _, id := range insertedIDs {
		expected[id] = true
	}
	for result := range results {
		count++

		if result.Err != nil {
			t.Errorf("result err:%v", result.Err)
		}
		if !result.Started {
			t.Errorf("start failed")
		}
		if u[result.TaskID] {
			t.Errorf("repeated id")

		} else {
			u[result.TaskID] = true
		}
		if !expected[result.TaskID] {
			t.Errorf("unexpected task id:%d", result.TaskID)
		}
	}

	if count != 5 {
		t.Fatalf("expect:5,get:%d", count)
	}
}
func TestRunPoolEmptyInput(t *testing.T) {
	ids := make(chan uint64)
	close(ids)
	results := RunPool(context.Background(), nil, ids, 3)
	select {
	case _, ok := <-results:
		if ok {
			t.Fatalf("get results")
		}
	case <-time.After(time.Second):
		t.Fatalf("time out")
	}

}
func TestRunPoolCancelWithoutClosingInput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ids := make(chan uint64)
	results := RunPool(ctx, nil, ids, 3)
	cancel()
	select {
	case _, ok := <-results:
		if ok {
			t.Fatalf("get results failed")
		}
	case <-time.After(time.Second):
		t.Fatal("worker pool did not stop")
	}
}

func TestRunPoolCancelWhileSending(t *testing.T) {
	db := openWorkerTestDB(t)

	result, err := db.Exec(`
    INSERT INTO tasks(task_type, payload)
    VALUES(?, ?)
`, "email", `{"to":"test@example.com"}`)

	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get id failed: %v", err)
	}

	taskID := uint64(id)
	t.Cleanup(func() {
		_, err := db.Exec("DELETE FROM tasks WHERE id=?", taskID)
		if err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ids := make(chan uint64, 1)
	ids <- taskID

	results := RunPool(ctx, db, ids, 1)

	var status string
	deadline := time.Now().Add(time.Second)

	for {
		err = db.QueryRowContext(ctx, `
        SELECT status FROM tasks WHERE id = ?
    `, taskID).Scan(&status)

		if err != nil {
			t.Fatalf("query failed: %v", err)
		}

		if status == "succeeded" {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("task was not claimed within 1 second")
		}

		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case _, ok := <-results:
		if ok {
			t.Fatalf("get results failed")
		}
	case <-time.After(time.Second):
		t.Fatal("worker pool did not stop")
	}

}
func TestRunPoolExecuteSuccess(t *testing.T) {

	db := openWorkerTestDB(t)
	insertResult, err := db.Exec(`
	INSERT INTO tasks(task_type,payload,status)
	VALUE(?,?,?)
	`, "email", `{"to":"test@example.com","simulate_failure":false}`, "pending")
	if err != nil {
		t.Fatalf("insert failed:%v", err)
	}
	idd, err := insertResult.LastInsertId()
	if err != nil {
		t.Fatalf("get last insert id failed")
	}
	if idd <= 0 {
		t.Fatalf("idd<=0")
	}
	t.Cleanup(func() {
		_, err := db.Exec(`
		DELETE FROM tasks
		WHERE id=?
		`, idd)
		if err != nil {
			t.Fatalf("delete failed:%v", err)
		}
	})
	ctx := context.Background()
	ids := make(chan uint64, 1)
	taskID := uint64(idd)
	ids <- taskID
	close(ids)
	results := RunPool(ctx, db, ids, 1)

	select {
	case r, ok := <-results:
		if !ok {
			t.Fatal("results closed unexpectedly")
		}
		if r.TaskID != taskID {
			t.Fatalf("expect taskid=%d,get:%d", r.TaskID, taskID)
		}
		if !r.Started {
			t.Fatalf("start failed")
		}
		if r.Err != nil {
			t.Fatalf("run failed:%v", r.Err)
		}

	case <-time.After(3 * time.Second):
		t.Fatal("worker timeout")
	}

	var status string
	var finishedAt sql.NullTime
	err = db.QueryRow(`
	SELECT status,finished_at
	FROM tasks
	WHERE id=?
	`, taskID).Scan(&status, &finishedAt)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if status != "succeeded" {
		t.Fatalf("expect:succeeded,get:%s", status)
	}
	if !finishedAt.Valid {
		t.Fatalf("finished at invalid")
	}
}
func TestRunPoolExecuteFailure(t *testing.T) {
	db := openWorkerTestDB(t)
	insertResult, err := db.Exec(`
	INSERT INTO tasks(task_type,payload,status)
	VALUES(?,?,?)
	`, "email", `{"to":"test@example.com","simulate_failure":true}`, "pending")
	if err != nil {
		t.Fatalf("insert failed:%v", err)
	}
	idd, err := insertResult.LastInsertId()
	if err != nil {
		t.Fatalf("get last insert id failed")
	}
	if idd <= 0 {
		t.Fatalf("idd<=0")
	}
	t.Cleanup(func() {
		_, err := db.Exec(`
		DELETE FROM tasks
		WHERE id=?
		`, idd)
		if err != nil {
			t.Fatalf("delete failed:%v", err)
		}
	})
	ctx := context.Background()

	ids := make(chan uint64, 1)
	taskID := uint64(idd)
	ids <- taskID
	close(ids)
	results := RunPool(ctx, db, ids, 1)
	select {
	case r, ok := <-results:
		if !ok {
			t.Fatal("results closed unexpectedly")
		}
		if r.TaskID != taskID {
			t.Fatalf("expect taskid=%d,get:%d", r.TaskID, taskID)
		}
		if !r.Started {
			t.Fatalf("start failed")
		}
		if r.Err == nil {
			t.Fatalf("run failed:%v", r.Err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("worker timeout")
	}
	var status string
	var finishedAt sql.NullTime
	var lastError sql.NullString
	err = db.QueryRow(`
	SELECT status,finished_at,last_error
	FROM tasks
	WHERE id=?
	`, taskID).Scan(&status, &finishedAt, &lastError)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if status != "failed" {
		t.Fatalf("expect status :failed,get : %s", status)
	}
	if !finishedAt.Valid {
		t.Fatalf("finished at invalid")
	}
	if !lastError.Valid || lastError.String == "" {
		t.Fatal("last_error is null or empty")
	}
}
