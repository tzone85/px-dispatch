package runtime

import (
	"fmt"
	"sort"

	"github.com/tzone85/px-dispatch/internal/agent"
	"github.com/tzone85/px-dispatch/internal/config"
	"github.com/tzone85/px-dispatch/internal/git"
	"github.com/tzone85/px-dispatch/internal/tmux"
)

// Router selects the best runtime for a given agent role based on
// configuration preferences, cost tier, capability matching, and health.
type Router struct {
	registry    *Registry
	preferences map[string]config.RoutingPreference // role -> preference
	strategy    string                              // cost_optimized | performance
	runner      git.CommandRunner                   // for health checks
}

// NewRouter creates a Router backed by the given registry and config.
// Preferences are indexed by role for O(1) lookup.
func NewRouter(reg *Registry, cfg config.Config) *Router {
	prefs := make(map[string]config.RoutingPreference, len(cfg.Routing.Preferences))
	for _, p := range cfg.Routing.Preferences {
		prefs[p.Role] = p
	}
	return &Router{
		registry:    reg,
		preferences: prefs,
		strategy:    cfg.Routing.Strategy,
	}
}

// NewRouterWithHealth creates a Router that performs health checks during selection.
func NewRouterWithHealth(reg *Registry, cfg config.Config, runner git.CommandRunner) *Router {
	r := NewRouter(reg, cfg)
	r.runner = runner
	return r
}

// SelectRuntime returns the best runtime for the given role.
// It checks configured preferences first (prefer -> fallback),
// then falls back to the first available runtime.
func (r *Router) SelectRuntime(role agent.Role) (Runtime, error) {
	// Check preference for this role.
	if pref, ok := r.preferences[string(role)]; ok {
		if rt, err := r.registry.Get(pref.Prefer); err == nil {
			return rt, nil
		}
		if pref.Fallback != "" {
			if rt, err := r.registry.Get(pref.Fallback); err == nil {
				return rt, nil
			}
		}
	}

	// Default: first available (sorted for determinism).
	names := r.registry.List()
	if len(names) == 0 {
		return nil, fmt.Errorf("no runtimes registered")
	}
	return r.registry.Get(names[0])
}

// SelectForModel returns the best runtime that supports the given model,
// preferring subscription-based runtimes when strategy is "cost_optimized".
func (r *Router) SelectForModel(model string) (Runtime, error) {
	candidates := r.runtimesSupportingModel(model)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no runtime supports model %q", model)
	}

	if r.strategy == "cost_optimized" {
		sortByCostTier(candidates)
	}

	return candidates[0], nil
}

// SelectHealthy returns the best healthy runtime for the given role.
// It tries the preferred runtime first, falling back through alternatives
// when the preferred runtime's session is unhealthy.
func (r *Router) SelectHealthy(role agent.Role, sessionName string) (Runtime, error) {
	// Build candidate list: preferred first, then fallback, then all others.
	candidates := r.buildCandidateList(role)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no runtimes registered")
	}

	// Without a runner, skip health checks and return first candidate.
	if r.runner == nil || sessionName == "" {
		return candidates[0], nil
	}

	// Try each candidate, skipping unhealthy ones.
	for _, rt := range candidates {
		result, err := rt.Health(r.runner, sessionName)
		if err != nil {
			continue
		}
		if result.Status == tmux.Healthy || result.Status == tmux.Missing {
			return rt, nil
		}
	}

	// All unhealthy — return first candidate anyway (caller decides).
	return candidates[0], nil
}

// buildCandidateList returns runtimes ordered by preference for the role.
func (r *Router) buildCandidateList(role agent.Role) []Runtime {
	seen := make(map[string]bool)
	var candidates []Runtime

	// Add preferred and fallback first.
	if pref, ok := r.preferences[string(role)]; ok {
		if rt, err := r.registry.Get(pref.Prefer); err == nil {
			candidates = append(candidates, rt)
			seen[rt.Name()] = true
		}
		if pref.Fallback != "" {
			if rt, err := r.registry.Get(pref.Fallback); err == nil && !seen[rt.Name()] {
				candidates = append(candidates, rt)
				seen[rt.Name()] = true
			}
		}
	}

	// Add remaining runtimes sorted by cost tier then name.
	remaining := make([]Runtime, 0)
	for _, name := range r.registry.List() {
		if !seen[name] {
			if rt, err := r.registry.Get(name); err == nil {
				remaining = append(remaining, rt)
			}
		}
	}
	if r.strategy == "cost_optimized" {
		sortByCostTier(remaining)
	}
	candidates = append(candidates, remaining...)

	return candidates
}

// runtimesSupportingModel returns all registered runtimes whose
// Capabilities().SupportsModel list contains the given model.
func (r *Router) runtimesSupportingModel(model string) []Runtime {
	var matches []Runtime
	for _, name := range r.registry.List() {
		rt, err := r.registry.Get(name)
		if err != nil {
			continue
		}
		for _, m := range rt.Capabilities().SupportsModel {
			if m == model {
				matches = append(matches, rt)
				break
			}
		}
	}
	return matches
}

// sortByCostTier sorts runtimes so subscription-based come before API-based.
func sortByCostTier(rts []Runtime) {
	sort.SliceStable(rts, func(i, j int) bool {
		return rts[i].Capabilities().CostTier < rts[j].Capabilities().CostTier
	})
}
