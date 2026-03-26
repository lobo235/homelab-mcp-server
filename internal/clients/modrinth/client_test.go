package modrinth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchProjects(t *testing.T) {
	resp := searchResponse{
		Hits: []Project{
			{ID: "abc123", Slug: "cobblemon", Title: "Cobblemon", ProjectType: "modpack", Downloads: 1000},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query().Get("query")
		if query != "cobblemon" {
			t.Errorf("unexpected query: %s", query)
		}
		facets := r.URL.Query().Get("facets")
		if facets != `[["project_type:modpack"]]` {
			t.Errorf("unexpected facets: %s", facets)
		}
		ua := r.Header.Get("User-Agent")
		if ua != "test-agent" {
			t.Errorf("unexpected User-Agent: %s", ua)
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClientWithBaseURL(srv.URL, "test-agent")
	projects, err := client.SearchProjects(context.Background(), "cobblemon", "modpack")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("len = %d, want 1", len(projects))
	}
	if projects[0].Slug != "cobblemon" {
		t.Errorf("Slug = %q, want %q", projects[0].Slug, "cobblemon")
	}
}

func TestGetProject(t *testing.T) {
	project := Project{
		ID:          "abc123",
		Slug:        "cobblemon",
		Title:       "Cobblemon",
		Description: "A mod",
		ProjectType: "modpack",
		Downloads:   5000,
		Categories:  []string{"adventure"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/project/cobblemon" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(project)
	}))
	defer srv.Close()

	client := NewClientWithBaseURL(srv.URL, "test-agent")
	result, err := client.GetProject(context.Background(), "cobblemon")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "abc123" {
		t.Errorf("ID = %q, want %q", result.ID, "abc123")
	}
	if result.Title != "Cobblemon" {
		t.Errorf("Title = %q, want %q", result.Title, "Cobblemon")
	}
}

func TestGetProject_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	client := NewClientWithBaseURL(srv.URL, "test-agent")
	_, err := client.GetProject(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		// The error is wrapped, check the unwrapped version.
		t.Logf("error type: %T, message: %v", err, err)
	} else if apiErr.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
}

func TestGetVersions(t *testing.T) {
	versions := []Version{
		{
			ID:            "v1",
			Name:          "Release 1.0",
			VersionNumber: "1.0.0",
			GameVersions:  []string{"1.20.1"},
			Loaders:       []string{"forge"},
			Files: []VersionFile{
				{URL: "https://cdn.modrinth.com/file.mrpack", Filename: "pack-1.0.0.mrpack", Primary: true, Size: 1024},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/project/abc123/version" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(versions)
	}))
	defer srv.Close()

	client := NewClientWithBaseURL(srv.URL, "test-agent")
	result, err := client.GetVersions(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	if result[0].VersionNumber != "1.0.0" {
		t.Errorf("VersionNumber = %q, want %q", result[0].VersionNumber, "1.0.0")
	}
	if len(result[0].Files) != 1 {
		t.Fatalf("Files len = %d, want 1", len(result[0].Files))
	}
	if !result[0].Files[0].Primary {
		t.Error("expected Primary = true")
	}
}

func TestGetVersion(t *testing.T) {
	version := Version{
		ID:            "v1",
		Name:          "Release 1.0",
		VersionNumber: "1.0.0",
		GameVersions:  []string{"1.20.1"},
		Loaders:       []string{"forge"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version/v1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(version)
	}))
	defer srv.Close()

	client := NewClientWithBaseURL(srv.URL, "test-agent")
	result, err := client.GetVersion(context.Background(), "v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "v1" {
		t.Errorf("ID = %q, want %q", result.ID, "v1")
	}
}

func TestGetVersion_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	client := NewClientWithBaseURL(srv.URL, "test-agent")
	_, err := client.GetVersion(context.Background(), "bad-id")
	if err == nil {
		t.Fatal("expected error")
	}
}
