package subscriptions

import (
	"context"
	"sync"
	"time"
)

const liveModelCatalogTTL = 10 * time.Minute

var modelCatalogCache struct {
	mu      sync.RWMutex
	expires time.Time
	models  []ModelInfo
}

func AvailableModels(ctx context.Context) ([]ModelInfo, error) {
	now := time.Now()

	modelCatalogCache.mu.RLock()
	if now.Before(modelCatalogCache.expires) && len(modelCatalogCache.models) > 0 {
		cached := cloneModelInfos(modelCatalogCache.models)
		modelCatalogCache.mu.RUnlock()
		return cached, nil
	}
	modelCatalogCache.mu.RUnlock()

	cfg, err := LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	upstreams, err := buildProviders(cfg)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	models := make([]ModelInfo, 0, len(upstreams)*4)
	for _, upstream := range upstreams {
		for _, model := range upstream.ListModels() {
			key := model.Provider + "\x00" + model.ID
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			models = append(models, model)
		}
	}

	modelCatalogCache.mu.Lock()
	modelCatalogCache.models = cloneModelInfos(models)
	modelCatalogCache.expires = now.Add(liveModelCatalogTTL)
	modelCatalogCache.mu.Unlock()

	return cloneModelInfos(models), nil
}

func cloneModelInfos(models []ModelInfo) []ModelInfo {
	if len(models) == 0 {
		return nil
	}
	cloned := make([]ModelInfo, len(models))
	copy(cloned, models)
	return cloned
}
