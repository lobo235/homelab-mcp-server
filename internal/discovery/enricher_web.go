package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const webSearchSystemPrompt = `You are a Minecraft server deployment expert. Search the web for server deployment guidance for the modpack described below.

CRITICAL OUTPUT RULES:
- Your ENTIRE response must be a single JSON object — nothing else
- Do NOT write any text before or after the JSON
- Do NOT use markdown code fences
- Start your response with { and end with }

JSON schema:
{"recommended_memory_mb": 0, "jvm_args": "", "known_issues": [], "config_overrides": {}, "mods_to_remove": [], "deployment_notes": "", "sources": []}

Field descriptions:
- recommended_memory_mb: integer, recommended RAM in MB (0 if unknown)
- jvm_args: string, recommended JVM arguments
- known_issues: array of strings, server-side issues and bugs
- config_overrides: object mapping filename to key-value pairs
- mods_to_remove: array of mod slug strings to remove for server deployment
- deployment_notes: string, important deployment guidance
- sources: array of source URL strings

Only include information from reliable sources. Do not fabricate data. Keep deployment_notes concise (under 2000 chars).`

// EnrichFromWeb uses the Anthropic API with web search to gather deployment
// knowledge for the modpack from the internet. Returns nil if the API key is
// empty or the search fails gracefully.
func (p *Pipeline) EnrichFromWeb(ctx context.Context, data *ExtractedData, anthropicKey string) (*WebSearchResults, error) {
	if anthropicKey == "" {
		p.Log.Info("skipping web search enrichment (no API key)")
		data.ConfidenceFlags = append(data.ConfidenceFlags, FlagWebSearchSkipped)
		return nil, nil
	}

	// Build the user prompt with modpack context.
	userPrompt := buildWebSearchPrompt(data)

	p.Log.Info("starting web search enrichment", "modpack", data.Name)

	responseText, err := callWebSearchWithRetry(ctx, anthropicKey, webSearchSystemPrompt, userPrompt)
	if err != nil {
		if strings.Contains(err.Error(), "authentication error") {
			p.Log.Error("web search authentication failed — check ANTHROPIC_API_KEY", "error", err)
		} else {
			p.Log.Warn("web search failed after retries", "error", err)
		}
		data.ConfidenceFlags = append(data.ConfidenceFlags, FlagWebSearchFailed)
		return nil, nil // Graceful failure — web search is optional.
	}

	// Save raw response for debugging if TempDir is set.
	if p.TempDir != "" {
		debugDir := filepath.Join(p.TempDir, "web_search_responses")
		if err := os.MkdirAll(debugDir, 0o700); err == nil {
			slug := strings.ReplaceAll(data.Name, " ", "_")
			_ = os.WriteFile(filepath.Join(debugDir, slug+".txt"), []byte(responseText), 0o600)
		}
	}

	// Parse the JSON response.
	results, err := parseWebSearchResponse(responseText)
	if err != nil {
		p.Log.Warn("failed to parse web search response", "error", err, "responseLen", len(responseText))
		data.ConfidenceFlags = append(data.ConfidenceFlags, FlagWebSearchFailed)
		return nil, nil
	}

	p.Log.Info("web search enrichment complete",
		"modpack", data.Name,
		"sources", len(results.Sources),
		"issues", len(results.KnownIssues),
	)

	return results, nil
}

// buildWebSearchPrompt creates the user prompt for web search based on extracted data.
func buildWebSearchPrompt(data *ExtractedData) string {
	var b strings.Builder

	fmt.Fprintf(&b, "I need server deployment information for the Minecraft modpack: %s\n\n", data.Name)

	if data.MinecraftVersion != "" {
		fmt.Fprintf(&b, "Minecraft version: %s\n", data.MinecraftVersion)
	}
	if data.ModloaderType != "" {
		fmt.Fprintf(&b, "Modloader: %s", data.ModloaderType)
		if data.ModloaderVersion != "" {
			fmt.Fprintf(&b, " %s", data.ModloaderVersion)
		}
		b.WriteString("\n")
	}
	if data.TotalModCount > 0 {
		fmt.Fprintf(&b, "Total mods: %d\n", data.TotalModCount)
	}

	if len(data.HeavyMods) > 0 {
		fmt.Fprintf(&b, "Notable heavy mods: %s\n", strings.Join(data.HeavyMods, ", "))
	}

	b.WriteString("\nPlease search for:\n")
	b.WriteString("1. Recommended RAM/memory allocation for this modpack's server\n")
	b.WriteString("2. Any recommended JVM arguments specific to this modpack\n")
	b.WriteString("3. Known server-side issues or bugs with this modpack version\n")
	b.WriteString("4. Any config file changes needed for dedicated server deployment\n")
	b.WriteString("5. Any mods that should be removed for server-side deployment\n")
	b.WriteString("6. Any special deployment notes or requirements\n")

	return b.String()
}

// parseWebSearchResponse extracts structured data from the LLM's JSON response.
func parseWebSearchResponse(text string) (*WebSearchResults, error) {
	// Clean up the response — the LLM might wrap in code fences or add preamble.
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	// If the response doesn't start with '{', try to find JSON object in the text.
	if !strings.HasPrefix(text, "{") {
		if start := strings.Index(text, "{"); start >= 0 {
			// Find the matching closing brace.
			if end := strings.LastIndex(text, "}"); end > start {
				text = text[start : end+1]
			}
		}
	}

	var results WebSearchResults
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		return nil, fmt.Errorf("parse web search JSON: %w", err)
	}

	return &results, nil
}
