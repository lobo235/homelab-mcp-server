package authz

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func makeReq(args map[string]any) mcp.CallToolRequest {
	raw, _ := json.Marshal(args)
	var req mcp.CallToolRequest
	_ = json.Unmarshal([]byte(`{"method":"tools/call","params":{"name":"test","arguments":`+string(raw)+`}}`), &req)
	return req
}

func TestExtractUserContext(t *testing.T) {
	tests := []struct {
		name           string
		args           map[string]any
		wantUserID     int64
		wantRole       string
		wantOwnedCount int
	}{
		{
			name:           "full context",
			args:           map[string]any{"_user_id": float64(42), "_user_role": "user", "_owned_servers": "mc-alpha,mc-beta"},
			wantUserID:     42,
			wantRole:       "user",
			wantOwnedCount: 2,
		},
		{
			name:           "admin empty owned",
			args:           map[string]any{"_user_id": float64(1), "_user_role": "admin", "_owned_servers": ""},
			wantUserID:     1,
			wantRole:       "admin",
			wantOwnedCount: 0,
		},
		{
			name:           "no context (legacy)",
			args:           map[string]any{"job_id": "mc-test"},
			wantUserID:     0,
			wantRole:       "",
			wantOwnedCount: 0,
		},
		{
			name:           "single owned server",
			args:           map[string]any{"_user_id": float64(5), "_user_role": "user", "_owned_servers": "mc-solo"},
			wantUserID:     5,
			wantRole:       "user",
			wantOwnedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := makeReq(tt.args)
			userID, role, owned := ExtractUserContext(req)
			if userID != tt.wantUserID {
				t.Errorf("userID = %d, want %d", userID, tt.wantUserID)
			}
			if role != tt.wantRole {
				t.Errorf("role = %q, want %q", role, tt.wantRole)
			}
			if len(owned) != tt.wantOwnedCount {
				t.Errorf("owned count = %d, want %d", len(owned), tt.wantOwnedCount)
			}
		})
	}
}

func TestRequireAdmin(t *testing.T) {
	tests := []struct {
		name    string
		role    string
		wantErr bool
	}{
		{"admin allowed", "admin", false},
		{"empty role allowed (legacy)", "", false},
		{"user denied", "user", true},
		{"kid denied", "kid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RequireAdmin(tt.role)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestRequireServerAccess(t *testing.T) {
	tests := []struct {
		name       string
		role       string
		serverName string
		owned      []string
		wantErr    bool
	}{
		{"admin bypasses", "admin", "mc-anything", nil, false},
		{"empty role allows (legacy)", "", "mc-test", nil, false},
		{"user owns server", "user", "mc-alpha", []string{"mc-alpha", "mc-beta"}, false},
		{"user does not own server", "user", "mc-gamma", []string{"mc-alpha", "mc-beta"}, true},
		{"user with no servers", "user", "mc-test", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RequireServerAccess(tt.role, tt.serverName, tt.owned)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestRequireJobAccess(t *testing.T) {
	tests := []struct {
		name    string
		role    string
		jobID   string
		owned   []string
		wantErr bool
	}{
		{"admin bypasses", "admin", "mc-anything", nil, false},
		{"empty role allows", "", "mc-test", nil, false},
		{"user owns job", "user", "mc-alpha", []string{"mc-alpha"}, false},
		{"user does not own job", "user", "mc-gamma", []string{"mc-alpha"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RequireJobAccess(tt.role, tt.jobID, tt.owned)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
