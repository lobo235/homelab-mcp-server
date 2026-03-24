package nomad

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lobo235/homelab-mcp-server/internal/clients"
)

func TestClient_ListJobs(t *testing.T) {
	jobs := []Job{
		{ID: "mc-atm10", Name: "mc-atm10", Type: "service", Status: "running"},
		{ID: "mc-vanilla1", Name: "mc-vanilla1", Type: "service", Status: "running"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jobs" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(jobs)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	result, err := client.ListJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if result[0].ID != "mc-atm10" {
		t.Errorf("result[0].ID = %q, want %q", result[0].ID, "mc-atm10")
	}
}

func TestClient_GetJobSpec(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jobs/mc-atm10/spec" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]string{"Source": "job \"mc-atm10\" { }"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	spec, err := client.GetJobSpec(context.Background(), "mc-atm10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec != "job \"mc-atm10\" { }" {
		t.Errorf("spec = %q, unexpected", spec)
	}
}

func TestClient_SubmitJob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/jobs" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) == 0 {
			t.Error("empty body in request")
		}
		if !strings.Contains(string(body), "job") {
			t.Error("body does not look like HCL")
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(SubmitResponse{JobID: "mc-new"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	resp, err := client.SubmitJob(context.Background(), "job \"mc-new\" { }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.JobID != "mc-new" {
		t.Errorf("JobID = %q, want %q", resp.JobID, "mc-new")
	}
}

func TestClient_StopJob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/jobs/mc-old" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.StopJob(context.Background(), "mc-old")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_ListAllocations(t *testing.T) {
	allocs := []Allocation{
		{ID: "alloc-1", Name: "mc-atm10.server[0]", ClientStatus: "running"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jobs/mc-atm10/allocations" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(allocs)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	result, err := client.ListAllocations(context.Background(), "mc-atm10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	if result[0].ClientStatus != "running" {
		t.Errorf("ClientStatus = %q, want %q", result[0].ClientStatus, "running")
	}
}

func TestClient_GetJobHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jobs/mc-atm10/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(HealthStatus{JobID: "mc-atm10", Healthy: true, Status: "running"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	health, err := client.GetJobHealth(context.Background(), "mc-atm10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !health.Healthy {
		t.Error("expected healthy")
	}
}

func TestClient_RestartAllocation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/jobs/mc-atm10/allocations/alloc-1/restart" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.RestartAllocation(context.Background(), "mc-atm10", "alloc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_GetJob(t *testing.T) {
	job := JobDetail{
		ID:          "mc-atm10",
		Name:        "mc-atm10",
		Type:        "service",
		Status:      "running",
		Datacenters: []string{"dc1"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/jobs/mc-atm10" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(job)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	result, err := client.GetJob(context.Background(), "mc-atm10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "mc-atm10" {
		t.Errorf("ID = %q, want %q", result.ID, "mc-atm10")
	}
	if result.Status != "running" {
		t.Errorf("Status = %q, want %q", result.Status, "running")
	}
}

func TestClient_GetAllocation(t *testing.T) {
	alloc := Allocation{ID: "alloc-1", Name: "mc-atm10.server[0]", ClientStatus: "running"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/jobs/mc-atm10/allocations/alloc-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(alloc)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	result, err := client.GetAllocation(context.Background(), "mc-atm10", "alloc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ClientStatus != "running" {
		t.Errorf("ClientStatus = %q, want %q", result.ClientStatus, "running")
	}
}

func TestClient_GetAllocation_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "allocation not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.GetAllocation(context.Background(), "mc-atm10", "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_GetAllocationLogs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/jobs/mc-atm10/allocations/alloc-1/logs" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		task := r.URL.Query().Get("task")
		if task != "server" {
			t.Errorf("task = %q, want %q", task, "server")
		}
		logType := r.URL.Query().Get("type")
		if logType != "stdout" {
			t.Errorf("type = %q, want %q", logType, "stdout")
		}
		w.Write([]byte("[INFO] Server started on port 25565\n[INFO] Done!"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	logs, err := client.GetAllocationLogs(context.Background(), "mc-atm10", "alloc-1", "server", "stdout")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(logs, "Server started") {
		t.Errorf("logs = %q, expected to contain 'Server started'", logs)
	}
}

func TestClient_GetAllocationLogs_DefaultLogType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logType := r.URL.Query().Get("type")
		if logType != "stdout" {
			t.Errorf("type = %q, want %q (default)", logType, "stdout")
		}
		w.Write([]byte("log output"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.GetAllocationLogs(context.Background(), "mc-atm10", "alloc-1", "server", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_GetAllocationLogs_URLEncoding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		task := r.URL.Query().Get("task")
		if task != "my task name" {
			t.Errorf("task = %q, want %q", task, "my task name")
		}
		w.Write([]byte("log output"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.GetAllocationLogs(context.Background(), "mc-atm10", "alloc-1", "my task name", "stderr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_GetAllocationLogs_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "allocation not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.GetAllocationLogs(context.Background(), "mc-atm10", "nonexistent", "server", "stdout")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_Ping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewClientWithBase(t *testing.T) {
	base := clients.NewBase("http://localhost:8080", "key")
	client := NewClientWithBase(base)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestClient_ListJobs_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "internal_error",
			"message": "upstream failure",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.ListJobs(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_GatewayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "job not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	_, err := client.GetJob(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}
