package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestGetTaskInvalidID(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/tasks/abc", nil)
	recorder := httptest.NewRecorder()
	mux := NewHandler(nil)
	mux.ServeHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("expect:400,get:%d", response.StatusCode)
	}
	m := make(map[string]string)
	err := json.NewDecoder(response.Body).Decode(&m)
	if err != nil {
		t.Fatalf("encode failed:%v", err)
	}
	if m["error"] != "invalid id" {
		t.Fatalf("expect err:invalid id,get:%s", m["error"])
	}
}
func TestGetTaskNotFound(t *testing.T) {
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
	err = db.Ping()
	if err != nil {
		t.Fatalf("mysql connect failed:%v", err)
	}

	var dbName string
	err = db.QueryRow(`
	SELECT DATABASE()
	`).Scan(&dbName)
	if err != nil {
		t.Fatalf("query database failed:%v", err)
	}
	if dbName != "go_task_platform_test" {
		t.Fatalf("expect:go_task_platform_test,got:%s", dbName)
	}
	var id uint64
	err = db.QueryRow(`
		SELECT id
		FROM tasks
		WHERE id=?
	`, 22).Scan(&id)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got:%v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/tasks/22", nil)
	recorder := httptest.NewRecorder()
	mux := NewHandler(db)
	mux.ServeHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expect:404,get:%d", response.StatusCode)
	}
	m := make(map[string]string)
	err = json.NewDecoder(response.Body).Decode(&m)
	if err != nil {
		t.Fatalf("decode failed:%v", err)
	}
	if m["error"] != "not found" {
		t.Fatalf("expect:not found,get:%s", m["error"])
	}
}
func TestGetTaskSuccess(t *testing.T) {
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
	err = db.Ping()
	if err != nil {
		t.Fatalf("mysql connect failed:%v", err)
	}
	var dbName string
	err = db.QueryRow(`
	SELECT DATABASE()
	`).Scan(&dbName)
	if err != nil {
		t.Fatalf("get dbName failed:%v", err)
	}
	if dbName != "go_task_platform_test" {
		t.Fatalf("expect :go_task_platform_test,get:%s", dbName)
	}

	result, err := db.Exec(`
	INSERT INTO tasks(task_type,payload)
	VALUES(?,?)
	`, "email", `{"to":"@example.com"}`)
	if err != nil {
		t.Fatalf("insert example failed:%v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get id failed:%v", err)
	}
	if id <= 0 {
		t.Fatalf("invalid inserted id:%d", id)
	}

	t.Cleanup(func() {
		_, err := db.Exec(`
		DELETE FROM tasks
		WHERE id=?
		`, id)
		if err != nil {
			t.Fatalf("delete failed:%v", err)
		}
	})
	url := fmt.Sprintf("/tasks/%d", id)
	request := httptest.NewRequest(http.MethodGet, url, nil)
	recorder := httptest.NewRecorder()
	mux := NewHandler(db)
	mux.ServeHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expect:statusok,get:%d", response.StatusCode)
	}
	contentType := response.Header.Get("Content-Type")

	if contentType != "application/json" {
		t.Fatalf("expect:Content-Type,got:%s", contentType)
	}
	var got struct {
		ID       uint64 `json:"id"`
		TaskType string `json:"task_type"`
		Status   string `json:"status"`
	}
	err = json.NewDecoder(response.Body).Decode(&got)
	if err != nil {
		t.Fatalf("decode failed:%v", err)
	}
	if got.TaskType != "email" {
		t.Fatalf("expect tasktype email,got:%s", got.TaskType)
	}
	if got.Status != "pending" {
		t.Fatalf("expect:pending,got:%s", got.Status)
	}
	if got.ID != uint64(id) {
		t.Fatalf("expected id:%d,got:%d", id, got.ID)
	}

}
func TestGetTaskDatabaseError(t *testing.T) {
	dsn := os.Getenv("GO_TASK_PLATFORM_TEST_DSN")
	if dsn == "" {
		t.Fatalf("get dsn failed")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("mysql open failed:%v", err)
	}
	db.Close()
	request := httptest.NewRequest(http.MethodGet, "/tasks/1", nil)
	recorder := httptest.NewRecorder()
	mux := NewHandler(db)
	mux.ServeHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expect:500,get:%d", response.StatusCode)
	}
	m := make(map[string]string)
	err = json.NewDecoder(response.Body).Decode(&m)
	if err != nil {
		t.Fatalf("decode failed:%v", err)
	}
	if m["error"] != "internal server error" {
		t.Fatalf("expect:internal server error,got:%s", m["error"])
	}
}
