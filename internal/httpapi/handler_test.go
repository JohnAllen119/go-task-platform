package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func openHTTPTestDB(t *testing.T) *sql.DB {
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
	return db
}
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
	db := openHTTPTestDB(t)
	var id uint64
	err := db.QueryRow(`
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
	db := openHTTPTestDB(t)

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

func TestCreateTaskSuccess(t *testing.T) {
	db := openHTTPTestDB(t)
	marker := fmt.Sprintf("test-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		result, err := db.Exec(`
		DELETE FROM tasks
		WHERE JSON_UNQUOTE(
				JSON_EXTRACT(payload,'$.test_marker')
		) =?;
		`, marker)
		if err != nil {
			t.Errorf("delete failed:%v", err)
			return
		}
		ra, err := result.RowsAffected()
		if err != nil {
			t.Errorf("get ra failed:%v", err)
			return
		}
		if ra != 1 {
			t.Errorf("expect:ra = 1,get:%d", ra)

		}
	})
	body := fmt.Sprintf(`{
    "task_type": "email",
    "payload": {
        "to": "test@example.com",
        "test_marker": %q
    }
}`, marker)
	request := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	mux := NewHandler(db)
	mux.ServeHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("expect :201,get:%d", response.StatusCode)
	}
	var taskResponse CreateTaskResponse
	err := json.NewDecoder(response.Body).Decode(&taskResponse)
	if err != nil {
		t.Fatalf("decode failed:%v", err)
	}
	if taskResponse.ID <= 0 {
		t.Fatalf("id <=0")
	}

	var taskType string
	var status string
	var tmarker string
	err = db.QueryRow(`
	SELECT task_type,status,JSON_UNQUOTE(JSON_EXTRACT(payload,'$.test_marker'))
	FROM tasks
	WHERE id=?
	`, taskResponse.ID).Scan(&taskType, &status, &tmarker)
	if err != nil {
		t.Fatalf("query failed:%v", err)
	}
	if taskType != "email" {
		t.Fatalf("expect:email,get:%s", taskType)
	}
	if status != "pending" {
		t.Fatalf("expect:pending,get:%s", status)
	}
	if tmarker != marker {
		t.Fatalf("expect:%s,get:%s", marker, tmarker)
	}
}

func TestCreateTaskInvalidJSON(t *testing.T) {
	body := `{
	"task_type" : "email",
	"payload" :{"to":}
	}`
	request := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(body))
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
		t.Fatalf("decode failed:%v", err)
	}
	if m["error"] != "decode failed" {
		t.Fatalf("expect decode failed,get:%s", m["error"])
	}
}
func TestCreateTaskInvalidTaskType(t *testing.T) {
	body := `{
	"task_type" : "",
	"payload" :{"to":"test@example.com"}
	}`

	mux := NewHandler(nil)
	request := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("expect:400,get:%d", response.StatusCode)
	}
	m := make(map[string]string)
	err := json.NewDecoder(response.Body).Decode(&m)
	if err != nil {
		t.Fatalf("decode response.body failed:%v", err)
	}
	if m["error"] != "invalid TaskType" {
		t.Fatalf("expect invalid TaskType,get:%s", m["error"])
	}
}
func TestCreateTaskMissingPayload(t *testing.T) {
	body := `{
	"task_type": "email"
	}`
	request := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(body))
	mux := NewHandler(nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("expect:400,get:%d", response.StatusCode)
	}
	contentType := response.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Fatalf("expect:application/json,get:%s", contentType)
	}
	m := make(map[string]string)
	err := json.NewDecoder(response.Body).Decode(&m)
	if err != nil {
		t.Fatalf("decode response body failed:%v", err)
	}
	if m["error"] != "invalid Payload" {
		t.Fatalf("expect invalid Payload,get:%s", m["error"])
	}
}
func TestCreateTaskDatabaseError(t *testing.T) {
	db := openHTTPTestDB(t)
	db.Close()
	body := `{
	"task_type":"email",
	"payload":{"to":"test@example.com"}
	}`
	request := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	mux := NewHandler(db)
	mux.ServeHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expect:500,get:%d", response.StatusCode)
	}
	contentType := response.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Fatalf("expect:application/json,get:%s", contentType)
	}
	m := make(map[string]string)
	err := json.NewDecoder(response.Body).Decode(&m)
	if err != nil {
		t.Fatalf("decode response body failed:%v", err)
	}
	if m["error"] != "create task failed" {
		t.Fatalf("expect create task failed,get:%s", m["error"])
	}
}
func TestCreateTaskNullPayload(t *testing.T) {
	body := `{
	"task_type" : "email",
	"payload" : null
	}`
	request := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(body))
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
		t.Fatalf("decode response body failed:%v", err)
	}
	if m["error"] != "invalid Payload" {
		t.Fatalf("expect:invalid Payload,get:%s", m["error"])
	}
}
func TestCreateTaskWhitespaceTaskType(t *testing.T) {
	body := `{
	"task_type":" ",
	"payload":{"message":"hello"}
	}`
	request := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(body))
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
		t.Fatalf("decode failed:%v", err)
	}
	if m["error"] != "invalid TaskType" {
		t.Fatalf("expect:invalid TaskTyp,get:%s", m["error"])
	}
}
