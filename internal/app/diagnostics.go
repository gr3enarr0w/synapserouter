package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gr3enarr0w/synapserouter/internal/router"
)

// DiagnosticCheck represents a single diagnostic result.
type DiagnosticCheck struct {
	Category string `json:"category"`
	Name     string `json:"name"`
	Status   string `json:"status"` // ok, warn, fail
	Message  string `json:"message"`
}

// RunDiagnostics performs a comprehensive health assessment.
func RunDiagnostics(ctx context.Context, ac *AppContext) []DiagnosticCheck {
	var checks []DiagnosticCheck

	checks = append(checks, checkEnvironment(ac)...)
	checks = append(checks, checkDatabase(ac)...)
	checks = append(checks, checkProviders(ctx, ac)...)
	checks = append(checks, checkCircuitBreakers(ac)...)
	checks = append(checks, checkUsageQuotas(ac)...)
	checks = append(checks, checkMemory(ac)...)

	return checks
}

func checkEnvironment(ac *AppContext) []DiagnosticCheck {
	var checks []DiagnosticCheck

	// ACTIVE_PROFILE
	profile := os.Getenv("ACTIVE_PROFILE")
	if profile == "" {
		checks = append(checks, DiagnosticCheck{
			Category: "environment",
			Name:     "ACTIVE_PROFILE",
			Status:   "warn",
			Message:  "Not set, defaulting to personal",
		})
	} else {
		checks = append(checks, DiagnosticCheck{
			Category: "environment",
			Name:     "ACTIVE_PROFILE",
			Status:   "ok",
			Message:  profile,
		})
	}

	// PORT
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090 (default)"
	}
	checks = append(checks, DiagnosticCheck{
		Category: "environment",
		Name:     "PORT",
		Status:   "ok",
		Message:  port,
	})

	return checks
}

func checkDatabase(ac *AppContext) []DiagnosticCheck {
	var checks []DiagnosticCheck

	if ac.DB == nil {
		checks = append(checks, DiagnosticCheck{
			Category: "database",
			Name:     "connection",
			Status:   "fail",
			Message:  "Database not initialized",
		})
		return checks
	}

	err := ac.DB.Ping()
	if err != nil {
		checks = append(checks, DiagnosticCheck{
			Category: "database",
			Name:     "connection",
			Status:   "fail",
			Message:  fmt.Sprintf("Ping failed: %v", err),
		})
		return checks
	}

	checks = append(checks, DiagnosticCheck{
		Category: "database",
		Name:     "connection",
		Status:   "ok",
		Message:  "Database reachable",
	})

	// Count tables
	var tableCount int
	err = ac.DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&tableCount)
	if err != nil {
		checks = append(checks, DiagnosticCheck{
			Category: "database",
			Name:     "tables",
			Status:   "warn",
			Message:  fmt.Sprintf("Could not count tables: %v", err),
		})
	} else {
		checks = append(checks, DiagnosticCheck{
			Category: "database",
			Name:     "tables",
			Status:   "ok",
			Message:  fmt.Sprintf("%d tables", tableCount),
		})
	}

	return checks
}

func checkProviders(ctx context.Context, ac *AppContext) []DiagnosticCheck {
	var checks []DiagnosticCheck

	if len(ac.Providers) == 0 {
		checks = append(checks, DiagnosticCheck{
			Category: "providers",
			Name:     "count",
			Status:   "warn",
			Message:  "No providers initialized",
		})
		return checks
	}

	checks = append(checks, DiagnosticCheck{
		Category: "providers",
		Name:     "count",
		Status:   "ok",
		Message:  fmt.Sprintf("%d providers loaded", len(ac.Providers)),
	})

	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	for _, p := range ac.Providers {
		healthy := p.IsHealthy(checkCtx)
		status := "ok"
		msg := "Healthy"
		if !healthy {
			status = "fail"
			msg = "Unhealthy"
		}
		checks = append(checks, DiagnosticCheck{
			Category: "providers",
			Name:     p.Name(),
			Status:   status,
			Message:  msg,
		})
	}

	return checks
}

func checkCircuitBreakers(ac *AppContext) []DiagnosticCheck {
	var checks []DiagnosticCheck

	if ac.DB == nil {
		return checks
	}

	states, err := router.GetAllCircuitStates(ac.DB)
	if err != nil {
		checks = append(checks, DiagnosticCheck{
			Category: "circuit_breakers",
			Name:     "states",
			Status:   "warn",
			Message:  fmt.Sprintf("Could not read states: %v", err),
		})
		return checks
	}

	for provider, state := range states {
		var status string
		switch state {
		case router.StateOpen:
			status = "fail"
		case router.StateHalfOpen:
			status = "warn"
		default:
			status = "ok"
		}
		checks = append(checks, DiagnosticCheck{
			Category: "circuit_breakers",
			Name:     provider,
			Status:   status,
			Message:  string(state),
		})
	}

	return checks
}

func checkUsageQuotas(ac *AppContext) []DiagnosticCheck {
	var checks []DiagnosticCheck

	if ac.UsageTracker == nil {
		return checks
	}

	quotas, err := ac.UsageTracker.GetAllQuotas()
	if err != nil {
		checks = append(checks, DiagnosticCheck{
			Category: "usage",
			Name:     "quotas",
			Status:   "warn",
			Message:  fmt.Sprintf("Could not read quotas: %v", err),
		})
		return checks
	}

	for name, quota := range quotas {
		status := "ok"
		msg := fmt.Sprintf("%.1f%% (%d/%d)", quota.UsagePercent*100, quota.CurrentUsage, quota.DailyLimit)
		if quota.UsagePercent >= 0.95 {
			status = "fail"
		} else if quota.UsagePercent >= 0.80 {
			status = "warn"
		}
		checks = append(checks, DiagnosticCheck{
			Category: "usage",
			Name:     name,
			Status:   status,
			Message:  msg,
		})
	}

	return checks
}

func checkMemory(ac *AppContext) []DiagnosticCheck {
	var checks []DiagnosticCheck

	if ac.DB == nil {
		return checks
	}

	var msgCount int
	err := ac.DB.QueryRow(`SELECT COUNT(*) FROM memory`).Scan(&msgCount)
	if err != nil {
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "no such table") {
			checks = append(checks, DiagnosticCheck{
				Category: "memory",
				Name:     "messages",
				Status:   "warn",
				Message:  "memory table not found",
			})
		} else {
			checks = append(checks, DiagnosticCheck{
				Category: "memory",
				Name:     "messages",
				Status:   "warn",
				Message:  fmt.Sprintf("Could not count: %v", err),
			})
		}
		return checks
	}

	var sessionCount int
	_ = ac.DB.QueryRow(`SELECT COUNT(DISTINCT session_id) FROM memory`).Scan(&sessionCount)

	checks = append(checks, DiagnosticCheck{
		Category: "memory",
		Name:     "messages",
		Status:   "ok",
		Message:  fmt.Sprintf("%d messages across %d sessions", msgCount, sessionCount),
	})

	return checks
}
