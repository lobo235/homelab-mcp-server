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
// Handles the mc-{name} / {name} convention: "survival" matches "mc-survival" and vice versa.
// Empty role means legacy/no context — access is allowed for backward compatibility.
func RequireServerAccess(role, serverName string, ownedServers []string) error {
	if role == "admin" || role == "" {
		return nil
	}
	if matchesOwned(serverName, ownedServers) {
		return nil
	}
	return fmt.Errorf("access denied: you don't have permission to access server %q", serverName)
}

// RequireJobAccess checks if the user has access to a Nomad job.
// Admins can access all jobs. Non-admins can only access jobs matching their owned servers.
// Handles the mc-{name} / {name} convention: "survival" matches "mc-survival" and vice versa.
// Empty role means legacy/no context — access is allowed for backward compatibility.
func RequireJobAccess(role, jobID string, ownedServers []string) error {
	if role == "admin" || role == "" {
		return nil
	}
	if matchesOwned(jobID, ownedServers) {
		return nil
	}
	return fmt.Errorf("access denied: you don't have permission to access job %q", jobID)
}

// matchesOwned checks if name matches any owned server, handling the mc- prefix convention.
// "survival" matches "mc-survival", "mc-survival" matches "mc-survival", etc.
func matchesOwned(name string, ownedServers []string) bool {
	// Generate variants: if name is "survival", also try "mc-survival"; if "mc-survival", also try "survival".
	variants := []string{name}
	if strings.HasPrefix(name, "mc-") {
		variants = append(variants, strings.TrimPrefix(name, "mc-"))
	} else {
		variants = append(variants, "mc-"+name)
	}

	for _, s := range ownedServers {
		for _, v := range variants {
			if s == v {
				return true
			}
		}
	}
	return false
}
