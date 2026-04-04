package marketplace

import (
	"fmt"
	"os"
)

// Integration represents an MCP server integration bundle
type Integration struct {
	Name           string
	Description    string
	MCPServer      string
	RequiredEnvVars []string
	Installed      bool
}

// integrationCatalog is the hardcoded catalog of available integrations
var integrationCatalog = []Integration{
	{
		Name:            "github",
		Description:     "GitHub MCP Server — repository access, issues, PRs, actions",
		MCPServer:       "github",
		RequiredEnvVars: []string{"GITHUB_TOKEN"},
	},
	{
		Name:            "slack",
		Description:     "Slack MCP Server — channels, messages, threads, files",
		MCPServer:       "slack",
		RequiredEnvVars: []string{"SLACK_TOKEN"},
	},
	{
		Name:            "jira",
		Description:     "Jira MCP Server — issues, projects, sprints, workflows",
		MCPServer:       "jira",
		RequiredEnvVars: []string{"JIRA_TOKEN", "JIRA_BASE_URL"},
	},
}

// ListIntegrations returns all available integrations with their installation status
func ListIntegrations() []Integration {
	result := make([]Integration, 0, len(integrationCatalog))
	for _, integ := range integrationCatalog {
		// Check if all required env vars are set
		installed := true
		for _, envVar := range integ.RequiredEnvVars {
			if os.Getenv(envVar) == "" {
				installed = false
				break
			}
		}
		integ.Installed = installed
		result = append(result, integ)
	}
	return result
}

// AddIntegration returns setup instructions for an integration
func AddIntegration(name string) (string, error) {
	// Find the integration in the catalog
	var target *Integration
	for i := range integrationCatalog {
		if integrationCatalog[i].Name == name {
			target = &integrationCatalog[i]
			break
		}
	}
	if target == nil {
		return "", fmt.Errorf("integration '%s' not found in catalog", name)
	}

	// Build setup instructions
	instructions := fmt.Sprintf(`# Integration: %s

%s

## Required Environment Variables

`, target.Name, target.Description)

	for _, envVar := range target.RequiredEnvVars {
		instructions += fmt.Sprintf("- `%s` — Set this in your .env file or shell\n", envVar)
	}

	instructions += `
## MCP Server Configuration

Add to your MCP config (typically ~/.synroute/mcp.json or your project's mcp.json):

`

	instructions += fmt.Sprintf(`
## Verification

After setting the environment variables, run:

  synroute integrations list

The '%s' integration should show as [installed].

## Next Steps

1. Set the required environment variables in your .env file
2. Restart synroute or reload your shell
3. Run 'synroute integrations list' to verify
`, target.Name)

	return instructions, nil
}

// GetCatalog returns the hardcoded integration catalog
func GetCatalog() []Integration {
	return integrationCatalog
}
