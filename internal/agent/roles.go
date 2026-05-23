// Package agent defines agent roles, complexity-based routing, and prompt
// generation for the px-dispatch multi-agent development system.
package agent

import "github.com/tzone85/px-dispatch/internal/config"

// Role represents an agent's role in the development team hierarchy.
type Role string

const (
	RoleTechLead     Role = "tech_lead"
	RoleSenior       Role = "senior"
	RoleIntermediate Role = "intermediate"
	RoleJunior       Role = "junior"
	RoleQA           Role = "qa"
	RoleSupervisor   Role = "supervisor"
)

// AllRoles returns all defined agent roles.
func AllRoles() []Role {
	return []Role{
		RoleTechLead,
		RoleSenior,
		RoleIntermediate,
		RoleJunior,
		RoleQA,
		RoleSupervisor,
	}
}

// RoleForComplexity returns the appropriate role based on Fibonacci complexity
// score and the routing thresholds in the configuration.
func RoleForComplexity(complexity int, routing config.RoutingConfig) Role {
	switch {
	case complexity <= routing.JuniorMaxComplexity:
		return RoleJunior
	case complexity <= routing.IntermediateMaxComplexity:
		return RoleIntermediate
	default:
		return RoleSenior
	}
}

// ModelConfig returns the model configuration for this role. Unknown roles
// fall back to the Junior model configuration.
func (r Role) ModelConfig(models config.ModelsConfig) config.ModelConfig {
	switch r {
	case RoleTechLead:
		return models.TechLead
	case RoleSenior:
		return models.Senior
	case RoleIntermediate:
		return models.Intermediate
	case RoleJunior:
		return models.Junior
	case RoleQA:
		return models.QA
	case RoleSupervisor:
		return models.Supervisor
	default:
		return models.Junior
	}
}
