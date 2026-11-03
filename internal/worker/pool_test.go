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
