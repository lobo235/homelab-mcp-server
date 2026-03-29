package atomic

import (
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/lobo235/homelab-mcp-server/internal/tools/authz"
)

// extractUserContext reads injected user context from a tool request.
func extractUserContext(req mcp.CallToolRequest) (int64, string, []string) {
	return authz.ExtractUserContext(req)
}

// requireServerAccess checks if the user has access to the given server.
func requireServerAccess(role, serverName string, ownedServers []string) error {
	return authz.RequireServerAccess(role, serverName, ownedServers)
}

// requireJobAccess checks if the user has access to a Nomad job.
func requireJobAccess(role, jobID string, ownedServers []string) error {
	return authz.RequireJobAccess(role, jobID, ownedServers)
}

// requireAdmin checks that the user has the admin role.
func requireAdmin(role string) error {
	return authz.RequireAdmin(role)
}
