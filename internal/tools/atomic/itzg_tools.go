package atomic

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func searchItzgDocs(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("search_itzg_docs",
			mcp.WithDescription("Search itzg/docker-minecraft-server documentation for a keyword. Returns matching lines with filenames. Use this to find the right env vars, configuration options, or server type documentation."),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("Keyword to search for (e.g., \"CURSEFORGE\", \"server.properties\", \"MEMORY\")"),
			),
		),
		Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := req.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			results := d.ItzgDocs.Search(query)
			data, err := json.Marshal(results)
			if err != nil {
				return mcp.NewToolResultError("marshal results: " + err.Error()), nil
			}
			return mcp.NewToolResultText(string(data)), nil
		},
	}
}

func getItzgDoc(d *Deps) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_itzg_doc",
			mcp.WithDescription("Read a specific itzg/docker-minecraft-server documentation page. Use search_itzg_docs first to find the right page."),
			mcp.WithString("filename",
				mcp.Required(),
				mcp.Description("Doc filename as returned by search (e.g., \"docs/configuration/server-properties.md\")"),
			),
		),
		Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			filename, err := req.RequireString("filename")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			content, err := d.ItzgDocs.Get(filename)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(content), nil
		},
	}
}
