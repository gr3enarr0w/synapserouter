package app

import (
	"context"
	"strings"

	"github.com/gr3enarr0w/synapserouter/internal/providers"
	"github.com/gr3enarr0w/synapserouter/internal/subscriptions"
)

// ListModels returns all available models from providers.
func ListModels(providerList []providers.Provider, profile, filterProvider string) []map[string]interface{} {
	seen := make(map[string]struct{})
	var out []map[string]interface{}

	// For personal profile, include subscription models
	if profile != "work" {
		models, err := subscriptions.AvailableModels(context.Background())
		if err == nil {
			for _, model := range models {
				if _, dup := seen[model.ID]; !dup {
					seen[model.ID] = struct{}{}
					out = append(out, map[string]interface{}{
						"id":       model.ID,
						"object":   model.Object,
						"owned_by": model.OwnedBy,
						"context":  model.Context,
					})
				}
			}
		}
	}

	// Merge models from registered providers
	for _, p := range providerList {
		if filterProvider != "" && !strings.EqualFold(p.Name(), filterProvider) {
			continue
		}
		if lm, ok := p.(interface{ ListModels() []map[string]interface{} }); ok {
			for _, m := range lm.ListModels() {
				id, _ := m["id"].(string)
				if _, dup := seen[id]; !dup && id != "" {
					seen[id] = struct{}{}
					m["provider"] = p.Name()
					out = append(out, m)
				}
			}
		}
	}

	return out
}
