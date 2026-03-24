// Package authz provides MCP-level authorization helpers for tool handlers.
// The chatbot injects _user_id, _user_role, and _owned_servers (comma-separated)
// into every tool call's arguments. These helpers extract and validate that context.
package authz

import (
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// ExtractUserContext reads injected user context from a tool request.
// Returns the user ID, role, and list of owned server names.
func ExtractUserContext(req mcp.CallToolRequest) (userID int64, role string, ownedServers []string) {
	args := req.GetArguments()

	if v, ok := args["_user_id"]; ok {
		// JSON numbers come through as float64.
		if f, ok := v.(float64); ok {
			userID = int64(f)
		}
	}

	role = req.GetString("_user_role", "")

	csv := req.GetString("_owned_servers", "")
	if csv != "" {
		ownedServers = strings.Split(csv, ",")
	}

	return
}

// RequireServerAccess checks if the user has access to the given server.
// Admins can access all servers. Non-admins can only access their owned servers.
// Empty role means legacy/no context — access is allowed for backward compatibility.
func RequireServerAccess(role, serverName string, ownedServers []string) error {
	if role == "admin" || role == "" {
		return nil
	}
	for _, s := range ownedServers {
		if s == serverName {
			return nil
		}
	}
	return fmt.Errorf("access denied: you don't have permission to access server %q", serverName)
}

// RequireJobAccess checks if the user has access to a Nomad job.
// Admins can access all jobs. Non-admins can only access jobs matching their owned servers.
// Empty role means legacy/no context — access is allowed for backward compatibility.
func RequireJobAccess(role, jobID string, ownedServers []string) error {
	if role == "admin" || role == "" {
		return nil
	}
	for _, s := range ownedServers {
		if s == jobID {
			return nil
		}
	}
	return fmt.Errorf("access denied: you don't have permission to access job %q", jobID)
}
