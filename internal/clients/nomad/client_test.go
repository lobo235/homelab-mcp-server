package nomad

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
		var req SubmitRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.HCL == "" {
			t.Error("empty HCL in request")
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
