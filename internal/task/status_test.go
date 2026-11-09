package task

import (
	"context"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"os"
	"testing"
)

func openStatusTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("GO_TASK_PLATFORM_TEST_DSN")
	if dsn == "" {
		t.Fatalf("get dsn failed")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("dsn open failed:%v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})
	err = db.Ping()
	if err != nil {
		t.Fatalf("mysql connect failed:%v", err)
	}
	var dbName string
	err = db.QueryRow(`
	SELECT DATABASE()
	`).Scan(&dbName)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if dbName != "go_task_platform_test" {
		t.Fatalf("expect:go_task_platform_test_dsn,get:%s", dbName)
	}
	return db
}
func TestMarkSucceeded(t *testing.T) {
	db := openStatusTestDB(t)
	ctx := context.Background()
	result, err := db.ExecContext(ctx, `
	INSERT INTO tasks(task_type,payload,status)
	VALUES(?,?,?)
	`, "email", `{"to":"test@example.com"}`, "pending")
	if err != nil {
		t.Fatalf("insert failed:%v", err)
	}

	idd, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get id failed:%v", err)
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
	tr, err := TryStartTask(ctx, db, id)
	if err != nil {
		t.Fatalf("trystarttask failed:%v", err)
	}
	if !tr {
		t.Fatalf("trystarttask,false")
	}
	tr, err = MarkSucceeded(ctx, db, id)
	if err != nil {
		t.Fatalf("mark succeeded failed:%v", err)
	}
	if !tr {
		t.Fatalf("mark succeeded false")
	}
	var status string
	var finishedAt sql.NullTime
	err = db.QueryRowContext(ctx, `
	SELECT status,finished_at
	FROM tasks
	WHERE id=?
	`, id).Scan(&status, &finishedAt)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if status != "succeeded" {
		t.Fatalf("expect status=succeeded,get:%s", status)
	}
	if !finishedAt.Valid {
		t.Fatalf("finishedAt==NULL")
	}
}
func TestMarkSucceededFromPending(t *testing.T) {
	db := openStatusTestDB(t)
	ctx := context.Background()
	result, err := db.ExecContext(ctx, `
	INSERT INTO tasks(task_type,payload,status)
	VALUES(?,?,?)
	`, "email", `{"to":"test@example.com"}`, "pending")
	if err != nil {
		t.Fatalf("insert task failed:%v", err)
	}
	idd, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get lastinsertid failed:%v", err)
	}
	if idd <= 0 {
		t.Fatalf("id<=0")
	}
	t.Cleanup(func() {
		_, err = db.Exec(`
		DELETE FROM tasks
		WHERE id=?
		`, idd)
		if err != nil {
			t.Fatalf("delete failed:%v", err)
		}
	})
	id := uint64(idd)
	updated, err := MarkSucceeded(ctx, db, id)
	if err != nil {
		t.Fatalf("marksucceeded failed:%v", err)
	}
	if updated {
		t.Fatalf("updated succeed")
	}
	var status string
	var finishedAt sql.NullTime
	err = db.QueryRowContext(ctx, `
	SELECT status,finished_at
	FROM tasks
	WHERE id=?
	`, idd).Scan(&status, &finishedAt)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if status != "pending" {
		t.Fatalf("expect:pending,get:%s", status)
	}
	if finishedAt.Valid {
		t.Fatalf("expect:finished_at=NULL")
	}
}
func TestMarkSucceededTwice(t *testing.T) {
	db := openStatusTestDB(t)
	ctx := context.Background()
	result, err := db.ExecContext(ctx, `
	INSERT INTO tasks(task_type,payload,status)
	VALUES(?,?,?)
	`, "email", `{"to":"test@example.com"}`, "pending")
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
	started, err := TryStartTask(ctx, db, id)
	if err != nil {
		t.Fatalf("try start task failed:%v", err)
	}
	if !started {
		t.Fatalf("started false")
	}
	updated, err := MarkSucceeded(ctx, db, id)
	if err != nil {
		t.Fatalf("marksucceed failed:%v", err)
	}
	if !updated {
		t.Fatalf("first updated failed")
	}
	var status string
	var FirstTime sql.NullTime
	err = db.QueryRowContext(ctx, `
	SELECT status,finished_at
	FROM tasks
	WHERE id=?
	`, idd).Scan(&status, &FirstTime)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	started, err = MarkSucceeded(ctx, db, id)
	if err != nil {
		t.Fatalf("starttask failed:%v", err)
	}
	if started {
		t.Fatalf("second updated succeed")
	}
	if !FirstTime.Valid {
		t.Fatalf("first time invalid")
	}
	var SecondTime sql.NullTime
	err = db.QueryRowContext(ctx, `
	SELECT status,finished_at
	FROM tasks
	WHERE id=?
	`, idd).Scan(&status, &SecondTime)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if status != "succeeded" {
		t.Fatalf("expect:succeed,expect:%s", status)
	}
	if !SecondTime.Valid {
		t.Fatalf("finishedAt==NULL")
	}
	if !FirstTime.Time.Equal(SecondTime.Time) {
		t.Fatalf("firsttime != secondTime")
	}
}
func TestMarkFailed(t *testing.T) {
	db := openStatusTestDB(t)
	ctx := context.Background()
	result, err := db.ExecContext(ctx, `
	INSERT INTO tasks(task_type,payload,status)
	VALUES(?,?,?)
	`, "email", `{"to":"test@example.com"}`, "pending")
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
	ts, err := TryStartTask(ctx, db, id)
	if err != nil {
		t.Fatalf("try start task failed:%v", err)
	}
	if !ts {
		t.Fatalf("try start task failed")
	}
	var status string
	err = db.QueryRowContext(ctx, `
	SELECT status
	FROM tasks
	WHERE id=?
	`, id).Scan(&status)
	if err != nil {
		t.Fatalf("select failed:%v", err)
	}
	if status != "running" {
		t.Fatalf("expect:running,get:%s", status)
	}
	mf, err := MarkFailed(ctx, db, id, "simulated execution failure")
	if err != nil {
		t.Fatalf("markfailed failed:%v", err)
	}
	if !mf {
		t.Fatalf("markfailed failed ")
	}
	var lastError sql.NullString
	var finishedAt sql.NullTime
	err = db.QueryRowContext(ctx, `
	SELECT status,last_error,finished_at
	FROM tasks
	WHERE id=?
	`, id).Scan(&status, &lastError, &finishedAt)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if status != "failed" {
		t.Fatalf("expect:failed,get:%s", status)
	}
	if lastError.String != "simulated execution failure" || !lastError.Valid {
		t.Fatalf("expect:simulated execution failure,get:%v", lastError)
	}
	if !finishedAt.Valid {
		t.Fatalf("finishedat=null")
	}
}
func TestMarkFailedAfterSucceeded(t *testing.T) {
	db := openStatusTestDB(t)
	ctx := context.Background()
	result, err := db.ExecContext(ctx, `
	INSERT INTO tasks(task_type,payload,status)
	VALUES(?,?,?)
	`, "email", `{"to":"test@example.com"}`, "pending")
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
	ts, err := TryStartTask(ctx, db, id)
	if err != nil {
		t.Fatalf("try start task failed:%v", err)
	}
	if !ts {
		t.Fatalf("try start task failed")
	}
	ms, err := MarkSucceeded(ctx, db, id)
	if err != nil {
		t.Fatalf("marksuccess failed:%v", err)
	}
	if !ms {
		t.Fatalf("marksuccess failed")
	}
	var status string
	var flastError sql.NullString
	var ffinishedAt sql.NullTime
	err = db.QueryRowContext(ctx, `
	SELECT status,last_error,finished_at
	FROM tasks
	WHERE id=?
	`, id).Scan(&status, &flastError, &ffinishedAt)
	if err != nil {
		t.Fatalf("select failed:%v", err)
	}
	if status != "succeeded" {
		t.Fatalf("expect:running,get:%s", status)
	}
	if flastError.Valid {
		t.Fatalf("last error valid")
	}
	if !ffinishedAt.Valid {
		t.Fatalf("finishedAt invalid")
	}

	mf, err := MarkFailed(ctx, db, id, "unexpected failure")
	if err != nil {
		t.Fatalf("markFailed failed:%v", err)
	}
	if mf {
		t.Fatalf("markedfailed success")
	}
	var slastError sql.NullString
	var sfinishedAt sql.NullTime
	err = db.QueryRowContext(ctx, `
	SELECT status,last_error,finished_at
	FROM tasks
	WHERE id=?
	`, id).Scan(&status, &slastError, &sfinishedAt)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if status != "succeeded" {
		t.Fatalf("expect:succeeded,get:%s", status)
	}
	if slastError.Valid {
		t.Fatalf("last error invalid")
	}
	if !sfinishedAt.Valid {
		t.Fatalf("finishedAt invalid")
	}
	if flastError.String != slastError.String {
		t.Fatalf("slast_error wrong")
	}
	if !ffinishedAt.Time.Equal(sfinishedAt.Time) {
		t.Fatalf("finished_at changed")
	}
}
