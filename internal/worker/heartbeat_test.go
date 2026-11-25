package worker

import (
	"context"
	"database/sql"
	"errors"
	"github.com/JohnAllen119/go-task-platform/internal/task"
	_ "github.com/go-sql-driver/mysql"
	"testing"
	"time"
)

func TestRunHeartbeatRenewsLease(t *testing.T) {
	db := openWorkerTestDB(t)
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
	tokenA, started, err := task.TryStartTask(ctx, db, id)
	if err != nil {
		t.Fatalf("try start task:%v", err)
	}
	if !started {
		t.Fatalf("start failed")
	}
	if tokenA == "" {
		t.Fatalf("tokenA is empty")
	}
	result, err = db.Exec(`
    UPDATE tasks
    SET lease_expires_at = NOW(6) + INTERVAL 2 SECOND
    WHERE id = ?
`, id)
	if err != nil {
		t.Fatalf("update failed:%v", err)
	}
	ra, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("get ra failed:%v", err)
	}
	if ra != 1 {
		t.Fatalf("ra!=1")
	}
	var oldTime sql.NullTime
	err = db.QueryRow(`
	SELECT lease_expires_at
	FROM tasks
	WHERE id=?
	`, id).Scan(&oldTime)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if !oldTime.Valid {
		t.Fatalf("oldtime invalid")
	}
	_, cancelExec := context.WithCancel(ctx)
	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	heartbeatDone := make(chan error, 1)
	t.Cleanup(func() {
		stopHeartbeat()
		heartbeatErr := <-heartbeatDone
		cancelExec()

		if heartbeatErr != nil {
			t.Errorf("heartbeat failed: %v", heartbeatErr)
		}
	})
	interval := 20 * time.Millisecond
	go func() {
		heartbeatDone <- runHeartbeat(heartbeatCtx, db, id, tokenA, interval, cancelExec, task.RenewLease)
	}()

	pollTicker := time.NewTicker(10 * time.Millisecond)
	defer pollTicker.Stop()

	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
waitForRenewal:
	for {
		select {
		case <-pollTicker.C:
			var newTime sql.NullTime
			err = db.QueryRow(`
		SELECT lease_expires_at
		FROM tasks
		WHERE id=?
		`, id).Scan(&newTime)
			if err != nil {
				t.Fatalf("query failed:%v", err)
			}
			if !newTime.Valid {
				t.Fatalf("newtime invalid")
			}
			if newTime.Time.Sub(oldTime.Time) > 20*time.Second {
				break waitForRenewal
			}

		case <-timeout.C:
			t.Fatal("heartbeat did not renew lease")
		}
	}
}
func TestRunHeartbeatLeaseLost(t *testing.T) {
	db := openWorkerTestDB(t)
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
	tokenA, started, err := task.TryStartTask(ctx, db, id)
	if err != nil {
		t.Fatalf("try start task:%v", err)
	}
	if !started {
		t.Fatalf("start failed")
	}
	if tokenA == "" {
		t.Fatalf("tokenA is empty")
	}
	execCtx, cancelExec := context.WithCancel(ctx)
	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	heartbeatDone := make(chan error, 1)
	t.Cleanup(func() {
		stopHeartbeat()
		heartbeatErr := <-heartbeatDone
		cancelExec()

		if heartbeatErr == nil {
			t.Errorf("expected heartbeat error after lease lost")
		}
	})
	interval := 20 * time.Millisecond
	go func() {
		heartbeatDone <- runHeartbeat(heartbeatCtx, db, id, tokenA, interval, cancelExec, task.RenewLease)
	}()

	result, err = db.Exec(`
	UPDATE tasks
	SET lease_token=?
	WHERE id=?
	`, "BBB", id)
	if err != nil {
		t.Fatalf("update failed:%v", err)
	}
	ra, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("get ra failed:%v", err)
	}
	if ra != 1 {
		t.Fatalf("ra!=1")
	}
	select {
	case <-execCtx.Done():
		return
	case <-time.After(2 * time.Second):
		t.Fatal("heartbeat did not cancel execCtx")
	}
}
func TestRunHeartbeatNormalStop(t *testing.T) {
	db := openWorkerTestDB(t)
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
	ctx := context.Background()
	id := uint64(idd)
	tokenA, started, err := task.TryStartTask(ctx, db, id)
	if err != nil {
		t.Fatalf("try start task:%v", err)
	}
	if !started {
		t.Fatalf("start failed")
	}
	if tokenA == "" {
		t.Fatalf("tokenA is empty")
	}
	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	execCtx, cancelExec := context.WithCancel(ctx)
	heartbeatDone := make(chan error, 1)
	t.Cleanup(cancelExec)
	go func() {
		heartbeatDone <- runHeartbeat(heartbeatCtx, db, id, tokenA, 20*time.Millisecond, cancelExec, task.RenewLease)
	}()
	stopHeartbeat()
	heartbeatErr := <-heartbeatDone
	if heartbeatErr != nil {
		t.Fatalf("heartbeatErr!=nil")
	}
	if execCtx.Err() != nil {
		t.Fatalf("execCtx was unexpectedly canceled: %v", execCtx.Err())
	}
}

func TestRunHeartbeatCancelDuringRenewal(t *testing.T) {
	started := make(chan struct{})
	fake := func(ctx context.Context,
		db *sql.DB,
		id uint64,
		leaseToken string) (bool, error) {
		close(started)
		<-ctx.Done()
		return false, context.Canceled
	}
	heartbeatDone := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	execCtx, cancelExec := context.WithCancel(context.Background())
	defer cancel()
	defer cancelExec()
	go func() {
		heartbeatDone <- runHeartbeat(ctx, nil, 0, "", 10*time.Millisecond, cancelExec, fake)
	}()
	select {
	case <-started:
		break
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout")
	}

	cancel()
	select {
	case heartbeatErr := <-heartbeatDone:
		if heartbeatErr != nil {
			t.Fatalf("runhertbeat failed:%v", heartbeatErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	if execCtx.Err() != nil {
		t.Fatalf("execCtx Err :=%v", execCtx.Err())
	}
}
func TestRunHeartbeatRenewError(t *testing.T) {
	wantErr := errors.New("mysql connection error")
	fake := func(ctx context.Context,
		db *sql.DB,
		id uint64,
		leaseToken string) (bool, error) {

		return false, wantErr
	}
	heartbeatDone := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	execCtx, cancelExec := context.WithCancel(context.Background())
	defer cancel()
	defer cancelExec()
	go func() {
		heartbeatDone <- runHeartbeat(ctx, nil, 0, "", 10*time.Millisecond, cancelExec, fake)
	}()
	select {
	case heartbeatErr := <-heartbeatDone:
		if !errors.Is(heartbeatErr, wantErr) {
			t.Fatalf("expect wantErr")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("time out")
	}
	if execCtx.Err() != context.Canceled {
		t.Fatalf("unexpected execCtx error: %v", execCtx.Err())
	}
}
func TestRunHeartbeatRenewRejected(t *testing.T) {

	fake := func(ctx context.Context,
		db *sql.DB,
		id uint64,
		leaseToken string) (bool, error) {
		return false, nil
	}
	heartbeatDone := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	execCtx, cancelExec := context.WithCancel(context.Background())
	defer cancel()
	defer cancelExec()
	go func() {
		heartbeatDone <- runHeartbeat(ctx, nil, 0, "", 10*time.Millisecond, cancelExec, fake)
	}()
	select {
	case heartbeatErr := <-heartbeatDone:
		if heartbeatErr == nil {
			t.Fatal("expected renewal rejection error")
		}
		if heartbeatErr.Error() != "lease renewal rejected for task 0" {
			t.Fatalf("unexpected error: %v", heartbeatErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("time out")
	}
	if execCtx.Err() != context.Canceled {
		t.Fatalf("unexpected execCtx error: %v", execCtx.Err())
	}
}
