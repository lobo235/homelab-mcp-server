package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const webSearchSystemPrompt = `You are a Minecraft server deployment expert. You will be given information about a modpack and asked to search the web for server deployment guidance.

Return your findings as a JSON object with this exact schema:
{
  "recommended_memory_mb": <integer or 0 if unknown>,
  "jvm_args": "<string or empty>",
  "known_issues": ["<issue descriptions>"],
  "config_overrides": {"<filename>": {"<key>": "<value>"}},
  "mods_to_remove": ["<mod slugs that should be removed for server-side>"],
  "deployment_notes": "<free text with any important deployment guidance>",
  "sources": ["<URLs of sources consulted>"]
}

Only include information you find from reliable sources. Do not fabricate data.
Return ONLY the JSON object, no markdown code fences or other text.`

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

	responseText, err := callWebSearch(ctx, anthropicKey, webSearchSystemPrompt, userPrompt)
	if err != nil {
		p.Log.Warn("web search failed", "error", err)
		data.ConfidenceFlags = append(data.ConfidenceFlags, FlagWebSearchFailed)
		return nil, nil // Graceful failure — web search is optional.
	}

	// Parse the JSON response.
	results, err := parseWebSearchResponse(responseText)
	if err != nil {
		p.Log.Warn("failed to parse web search response", "error", err)
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
	// Clean up the response — the LLM might wrap in code fences.
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var results WebSearchResults
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		return nil, fmt.Errorf("parse web search JSON: %w", err)
	}

	return &results, nil
}
