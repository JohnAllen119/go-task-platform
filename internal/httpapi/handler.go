package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/JohnAllen119/go-task-platform/internal/task"
	"net/http"
	"strconv"
	"strings"
)

type ErrorResponse struct {
	Error string `json:"error"`
}
type TaskResponse struct {
	ID       uint64 `json:"id"`
	TaskType string `json:"task_type"`
	Status   string `json:"status"`
}
type CreateTaskRequest struct {
	TaskType string          `json:"task_type"`
	Payload  json.RawMessage `json:"payload"`
}
type CreateTaskResponse struct {
	ID uint64 `json:"id"`
}

func NewHandler(db *sql.DB) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		taskByIDHandler(db, w, r)
	})
	mux.HandleFunc("POST /tasks", func(w http.ResponseWriter, r *http.Request) {
		CreateTaskHandler(db, w, r)
	})
	return mux
}
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}
func taskByIDHandler(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	rawID := r.PathValue("id")
	id, err := strconv.ParseUint(rawID, 10, 64)
	if id <= 0 || err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ctx := r.Context()
	foundtask, err := task.FindByID(ctx, db, id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	var taskResponse TaskResponse
	taskResponse.ID = foundtask.ID
	taskResponse.TaskType = foundtask.TaskType
	taskResponse.Status = foundtask.Status
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(&taskResponse)
}
func CreateTaskHandler(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var taskRequest CreateTaskRequest
	err := json.NewDecoder(r.Body).Decode(&taskRequest)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "decode failed")
		return
	}
	taskRequest.TaskType = strings.TrimSpace(taskRequest.TaskType)
	if taskRequest.Payload == nil {
		writeJSONError(w, http.StatusBadRequest, "invalid Payload")
		return
	}
	if taskRequest.TaskType == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid TaskType")
		return
	}
	if string(taskRequest.Payload) == "null" {
		writeJSONError(w, http.StatusBadRequest, "invalid Payload")
		return
	}
	ctx := r.Context()
	newID, err := task.CreateTask(ctx, db, taskRequest.TaskType, taskRequest.Payload)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "create task failed")
		return
	}
	var taskResponse CreateTaskResponse
	taskResponse.ID = newID
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(taskResponse)

}
