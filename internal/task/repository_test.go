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
	token, est, err := TryStartTask(ctx, db, uint64(lid))

	if err != nil {
		t.Fatalf("trystartedtask failed")
	}
	if est != true {
		t.Fatalf("start failed")
	}
	if token == "" {
		t.Fatalf("token is empty")
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
	token, tst, err := TryStartTask(ctx, db, uint64(ra))
	if err != nil {
		t.Fatalf("trystarttask failed:%v", err)
	}
	if !tst {
		t.Fatalf("start task filed")
	}
	if token == "" {
		t.Fatalf("token is empty")
	}
	token, tst, err = TryStartTask(ctx, db, uint64(ra))
	if err != nil {
		t.Fatalf("trystarttask failed:%v", err)
	}
	if tst {
		t.Fatalf("start task shouldn't succeed")
	}
	if token != "" {
		t.Fatalf("token shoudn't exist")
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

			_, est, err := TryStartTask(ctx, db, uint64(ra))
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

func TestTryStartTaskLease(t *testing.T) {
	db := openTestDB(t)
	result, err := db.Exec(`
INSERT INTO tasks(task_type,payload)
VALUES(?,?)
`, "email", `{"to":"test@example.com"}`)
	if err != nil {
		t.Fatalf("insert failed:%v", err)
	}
	idd, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get last insert id failed:%v", err)
	}
	if idd <= 0 {
		t.Fatalf("id <=0 ")
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
	var leaseExpiresAt sql.NullTime
	err = db.QueryRow(`
	SELECT lease_expires_at
	FROM tasks
	WHERE id=?
	`, idd).Scan(&leaseExpiresAt)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if leaseExpiresAt.Valid {
		t.Fatal("pending task should not have a lease")
	}

	id := uint64(idd)
	ctx := context.Background()
	token, started, err := TryStartTask(ctx, db, id)
	if err != nil {
		t.Fatalf("try start task failed:%v", err)
	}
	if !started {
		t.Fatalf("try start task failed")
	}
	if token == "" {
		t.Fatalf("token is empty")
	}
	err = db.QueryRow(`
	SELECT lease_expires_at
	FROM tasks
	WHERE id=?
	`, idd).Scan(&leaseExpiresAt)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if !leaseExpiresAt.Valid {
		t.Fatal("lease_expires_at invalid")
	}
	l := leaseExpiresAt
	token, started, err = TryStartTask(ctx, db, id)
	if err != nil {
		t.Fatalf("try start task failed:%v", err)
	}
	if started {
		t.Fatalf("try start task succeed")
	}
	if token != "" {
		t.Fatalf("token is not empty")
	}
	var ti int64
	err = db.QueryRow(`
	SELECT lease_expires_at,TIMESTAMPDIFF(
    MICROSECOND,
    NOW(6),
    lease_expires_at
)
	FROM tasks
	WHERE id=?
	`, idd).Scan(&leaseExpiresAt, &ti)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if !leaseExpiresAt.Valid {
		t.Fatal("lease_expires_at invalid")
	}
	if !l.Time.Equal(leaseExpiresAt.Time) {
		t.Fatal("lease expiry changed after duplicate claim")
	}
	if ti < 25000000 || ti > 30000000 {
		t.Fatal("lease duration invalid")
	}

}

func TestMarkSucceededWrongLeaseToken(t *testing.T) {
	db := openTestDB(t)
	result, err := db.Exec(`
INSERT INTO tasks(task_type,payload)
VALUES(?,?)
`, "email", `{"to":"test@example.com"}`)
	if err != nil {
		t.Fatalf("insert failed:%v", err)
	}
	idd, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get last insert id failed:%v", err)
	}
	if idd <= 0 {
		t.Fatalf("id <=0 ")
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
	id := uint64(idd)
	token, started, err := TryStartTask(ctx, db, id)
	if err != nil {
		t.Fatalf("try start task failed:%v", err)
	}
	if !started {
		t.Fatalf("try start task failed")
	}
	if token == "" {
		t.Fatalf("token is empty")
	}
	ms, err := MarkSucceeded(ctx, db, id, "BBB")
	if err != nil {
		t.Fatalf("mark should failed:%v", err)
	}
	if ms {
		t.Fatalf("mark should not succeed")
	}
	var status string
	err = db.QueryRow(`
	SELECT status
	FROM tasks
	WHERE id=?
	`, id).Scan(&status)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if status != "running" {
		t.Fatalf("expect status:running,got:%s", status)
	}
	ms, err = MarkSucceeded(ctx, db, id, token)
	if err != nil {
		t.Fatalf("mark failed:%v", err)
	}
	if !ms {
		t.Fatalf("mark not succeed")
	}
	err = db.QueryRow(`
	SELECT status
	FROM tasks
	WHERE id=?
	`, id).Scan(&status)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if status != "succeeded" {
		t.Fatalf("expect:succeeded,get:%s", status)
	}
}

func TestTryStartTaskExpiredLease(t *testing.T) {
	db := openTestDB(t)
	result, err := db.Exec(`
INSERT INTO tasks(task_type,payload)
VALUES(?,?)
`, "email", `{"to":"test@example.com"}`)
	if err != nil {
		t.Fatalf("insert failed:%v", err)
	}
	idd, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get last insert id failed:%v", err)
	}
	if idd <= 0 {
		t.Fatalf("id <=0 ")
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
	id := uint64(idd)
	tokenA, started, err := TryStartTask(ctx, db, id)
	if err != nil {
		t.Fatalf("try start failed:%v", err)
	}
	if !started {
		t.Fatalf("started failed")
	}
	if tokenA == "" {
		t.Fatalf("tokenA is empty")
	}
	_, err = db.Exec(`
	UPDATE tasks
SET lease_expires_at = NOW(6) - INTERVAL 1 SECOND
WHERE id = ?;
	`, idd)
	if err != nil {
		t.Fatalf("update lease_expires failed:%v", err)
	}
	tokenB, started, err := TryStartTask(ctx, db, id)
	if err != nil {
		t.Fatalf("try start failed:%v", err)
	}
	if !started {
		t.Fatalf("started failed")
	}
	if tokenB == "" {
		t.Fatalf("tokenB is empty")
	}
	if tokenB == tokenA {
		t.Fatalf("tokenB=tokenA")
	}
	var lt string
	err = db.QueryRow(`
	SELECT lease_token
	FROM tasks
	WHERE id = ?;
	`, id).Scan(&lt)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if lt != tokenB {
		t.Fatalf("lease_token!=tokenB")
	}
	ms, err := MarkSucceeded(ctx, db, id, tokenA)
	if err != nil {
		t.Fatalf("mark succeeded error:%v", err)
	}
	if ms {
		t.Fatalf("ms should not succeeded")
	}
	var status string
	err = db.QueryRow(`
	SELECT status
	FROM tasks
	WHERE id=?
	`, id).Scan(&status)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if status != "running" {
		t.Fatalf("expect :running,get:%s", status)
	}
	ms, err = MarkSucceeded(ctx, db, id, tokenB)
	if err != nil {
		t.Fatalf("mark succeeded error:%v", err)
	}
	if !ms {
		t.Fatalf("ms should succeeded")
	}
	err = db.QueryRow(`
	SELECT status
	FROM tasks
	WHERE id=?
	`, id).Scan(&status)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if status != "succeeded" {
		t.Fatalf("expect :succeeded,get:%s", status)
	}
}

func TestTryStartTaskExpiredLeaseConcurrent(t *testing.T) {
	db := openTestDB(t)
	result, err := db.Exec(`
	INSERT INTO tasks(task_type,payload,lease_expires_at,status)
	VALUES(?,?,NOW(6)-INTERVAL 1 SECOND,?)
	`, "email", `{"to":"test@example.com"}`, "running")
	if err != nil {
		t.Fatalf("insert failed:%v", err)
	}
	idd, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get lastinsertid failed:%v", err)
	}
	if idd <= 0 {
		t.Fatalf("idd<=0")
	}
	t.Cleanup(
		func() {
			_, err := db.Exec(`
			DELETE FROM tasks
			WHERE id=?
			`, idd)
			if err != nil {
				t.Fatalf("delete failed:%v", err)
			}
		})
	var wg sync.WaitGroup
	wg.Add(2)
	start := make(chan struct{})
	ctx := context.Background()
	id := uint64(idd)
	var tokenA string
	var tokenB string
	var startedA bool
	var startedB bool
	var errA error
	var errB error
	ready := make(chan struct{}, 2)
	go func() {
		defer wg.Done()
		ready <- struct{}{}
		<-start
		tokenA, startedA, errA = TryStartTask(ctx, db, id)

	}()

	go func() {
		defer wg.Done()
		ready <- struct{}{}
		<-start
		tokenB, startedB, errB = TryStartTask(ctx, db, id)

	}()
	<-ready
	<-ready
	close(start)
	wg.Wait()
	if errA != nil {
		t.Fatalf("start task failed:%v", errA)
	}
	if errB != nil {
		t.Fatalf("start task failed:%v", errB)
	}
	if startedA {
		if startedB {
			t.Fatalf("should not be two true")
		}
	} else {
		if !startedB {
			t.Fatalf("should not be two false")
		}
	}
	var token string
	if tokenA == "" && tokenB == "" {
		t.Fatalf("tokenA and tokenB are both empty")
	}
	if tokenA != "" && tokenB != "" {
		t.Fatalf("tokenA and tokenB are both filled")
	}
	if tokenA != "" {
		token = tokenA
	} else {
		token = tokenB
	}
	var leaseToken string
	var status string
	err = db.QueryRow(`
	SELECT lease_token ,status
	FROM tasks
	WHERE id=?
	`, id).Scan(&leaseToken, &status)
	if err != nil {
		t.Fatalf("query failed :%v", err)
	}
	if leaseToken != token {
		t.Fatalf("leaseToken!=token")
	}
	if status != "running" {
		t.Fatalf("expect status:running,get:%s", status)
	}
}
func TestRenewLeaseSuccess(t *testing.T) {
	db := openTestDB(t)
	result, err := db.Exec(`
	INSERT INTO tasks(task_type,payload)
	VALUES(?,?)
	`, "email", `{"to":"test@example.com"}`)
	if err != nil {
		t.Fatalf("insert failed:%v", err)
	}
	idd, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get last insert id failed:%v", err)
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
	id := uint64(idd)
	ctx := context.Background()
	leaseToken, started, err := TryStartTask(ctx, db, id)
	if err != nil {
		t.Fatalf("try start task failed:%v", err)
	}
	if !started {
		t.Fatalf("start task failed")
	}
	if leaseToken == "" {
		t.Fatalf("leaseToken is empty")
	}
	var oldtime sql.NullTime
	err = db.QueryRow(`
	SELECT lease_expires_at
	FROM tasks
	WHERE id=?
	`, id).Scan(&oldtime)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if !oldtime.Valid {
		t.Fatalf("old time invalid")
	}
	renew, err := RenewLease(ctx, db, id, leaseToken)
	if err != nil {
		t.Fatalf("renewlease failed:%v", err)
	}
	if !renew {
		t.Fatalf("renew lease failed")
	}
	var newtime sql.NullTime
	err = db.QueryRow(`
	SELECT lease_expires_at
	FROM tasks
	WHERE id=?
	`, id).Scan(&newtime)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if !newtime.Valid {
		t.Fatalf("new time invalid")
	}
	if !newtime.Time.After(oldtime.Time) {
		t.Fatalf(
			"lease was not extended: old=%v, new=%v",
			oldtime.Time,
			newtime.Time,
		)
	}
}
func TestRenewLeaseWrongToken(t *testing.T) {
	db := openTestDB(t)
	result, err := db.Exec(`
	INSERT INTO tasks(task_type,payload)
	VALUES(?,?)
	`, "email", `{"to":"test@example.com"}`)
	if err != nil {
		t.Fatalf("insert failed:%v", err)
	}
	idd, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get last insert id failed:%v", err)
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
	id := uint64(idd)
	ctx := context.Background()
	TokenA, started, err := TryStartTask(ctx, db, id)
	if err != nil {
		t.Fatalf("try start task failed:%v", err)
	}
	if !started {
		t.Fatalf("start task failed")
	}
	if TokenA == "" {
		t.Fatalf("TokenA is empty")
	}
	var oldtime sql.NullTime
	err = db.QueryRow(`
	SELECT lease_expires_at
	FROM tasks
	WHERE id=?
	`, id).Scan(&oldtime)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if !oldtime.Valid {
		t.Fatalf("old time invalid")
	}
	renew, err := RenewLease(ctx, db, id, "AAA")
	if err != nil {
		t.Fatalf("renewlease failed:%v", err)
	}
	if renew {
		t.Fatalf("renew should not succeeded")
	}
	var newtime sql.NullTime
	err = db.QueryRow(`
	SELECT lease_expires_at
	FROM tasks
	WHERE id=?
	`, id).Scan(&newtime)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if !newtime.Valid {
		t.Fatalf("new time invalid")
	}
	if !oldtime.Time.Equal(newtime.Time) {
		t.Fatalf("lease_time_at changed")
	}
	var token string
	err = db.QueryRow(`
	SELECT lease_token
	FROM tasks
	WHERE id=?
	`, id).Scan(&token)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if token != TokenA {
		t.Fatalf("token != tokenA")
	}
}
func TestRenewLeaseExpired(t *testing.T) {
	db := openTestDB(t)
	result, err := db.Exec(`
	INSERT INTO tasks(task_type,payload)
	VALUES(?,?)
	`, "email", `{"to":"test@example.com"}`)
	if err != nil {
		t.Fatalf("insert failed:%v", err)
	}
	idd, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get last insert id failed:%v", err)
	}
	if idd <= 0 {
		t.Fatalf("idd <=0")
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
	id := uint64(idd)
	ctx := context.Background()
	tokenA, started, err := TryStartTask(ctx, db, id)
	if err != nil {
		t.Fatalf("try start task failed:%v", err)
	}
	if !started {
		t.Fatalf("try start task failed")
	}
	if tokenA == "" {
		t.Fatalf("tokenA is empty")
	}
	result, err = db.Exec(`
	UPDATE tasks
	SET lease_expires_at=Now(6)- INTERVAL 1 SECOND
	WHERE id=?
	`, id)
	if err != nil {
		t.Fatalf("update lease_expires_at failed:%v", err)
	}
	ra, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("get rows affect failed:%v", err)
	}
	if ra != 1 {
		t.Fatalf("rowsaffect != 1")
	}
	var tr bool
	err = db.QueryRow(`
		SELECT lease_expires_at <= NOW(6)
		FROM tasks
		WHERE id=?
	`, id).Scan(&tr)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if !tr {
		t.Fatalf("lease_expires_at > now")
	}
	var oldTime sql.NullTime
	err = db.QueryRow(`
	SELECT lease_expires_at
	FROM tasks
	WHERE id=?
	`, id).Scan(&oldTime)
	if err != nil {
		t.Fatalf("qeury failed:%v", err)
	}
	if !oldTime.Valid {
		t.Fatalf("old time invalid")
	}

	renew, err := RenewLease(ctx, db, id, tokenA)
	if err != nil {
		t.Fatalf("renewlease failed:%v", err)
	}
	if renew {
		t.Fatalf("renew should not be succeeded")
	}
	var newTime sql.NullTime
	err = db.QueryRow(`
	SELECT lease_expires_at
	FROM tasks
	WHERE id=?
	`, id).Scan(&newTime)
	if err != nil {
		t.Fatalf("qeury failed:%v", err)
	}
	if !newTime.Valid {
		t.Fatalf("newtime invalid")
	}
	if !newTime.Time.Equal(oldTime.Time) {
		t.Fatalf("renewlease succeed")
	}
}
func TestRenewLeaseAfterSucceeded(t *testing.T) {
	db := openTestDB(t)
	result, err := db.Exec(`
	INSERT INTO tasks(task_type,payload)
	VALUES(?,?)
	`, "email", `{"to":"test@example.com"}`)
	if err != nil {
		t.Fatalf("insert failed:%v", err)
	}
	idd, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get last insert id failed:%v", err)
	}
	if idd <= 0 {
		t.Fatalf("id<=0")
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
	id := uint64(idd)
	tokenA, started, err := TryStartTask(ctx, db, id)
	if err != nil {
		t.Fatalf("try start task failed:%v", err)
	}
	if !started {
		t.Fatalf("start failed")
	}
	if tokenA == "" {
		t.Fatalf("tokenA is empty")
	}
	ms, err := MarkSucceeded(ctx, db, id, tokenA)
	if err != nil {
		t.Fatalf("marksucceeded failed:%v", err)
	}
	if !ms {
		t.Fatalf("mark succeeded failed")
	}
	var status string
	var oldTime sql.NullTime
	err = db.QueryRow(`
	SELECT status , lease_expires_at
	FROM tasks
	WHERE id=?
	`, id).Scan(&status, &oldTime)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if status != "succeeded" {
		t.Fatalf("expect:succeeded,get:%s", status)
	}
	if !oldTime.Valid {
		t.Fatalf("invalid old time")
	}
	renew, err := RenewLease(ctx, db, id, tokenA)
	if err != nil {
		t.Fatalf("renewlease error:%v", err)
	}
	if renew {
		t.Fatalf("renewlease should not be succeeded")
	}
	var newTime sql.NullTime
	err = db.QueryRow(`
	SELECT status , lease_expires_at
	FROM tasks
	WHERE id=?
	`, id).Scan(&status, &newTime)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if status != "succeeded" {
		t.Fatalf("expect:succeeded,get:%s", status)
	}
	if !newTime.Valid {
		t.Fatalf("invalid new time")
	}
	if !oldTime.Time.Equal(newTime.Time) {
		t.Fatalf("oldtime!=newtime")
	}
}
