package agent

import (
	"testing"

	"github.com/tzone85/px-dispatch/internal/config"
)

func TestRoleForComplexity_Junior(t *testing.T) {
	cfg := config.RoutingConfig{JuniorMaxComplexity: 3, IntermediateMaxComplexity: 5}
	role := RoleForComplexity(2, cfg)
	if role != RoleJunior {
		t.Errorf("complexity 2 should be Junior, got %s", role)
	}
}

func TestRoleForComplexity_Intermediate(t *testing.T) {
	cfg := config.RoutingConfig{JuniorMaxComplexity: 3, IntermediateMaxComplexity: 5}
	role := RoleForComplexity(4, cfg)
	if role != RoleIntermediate {
		t.Errorf("complexity 4 should be Intermediate, got %s", role)
	}
}

func TestRoleForComplexity_Senior(t *testing.T) {
	cfg := config.RoutingConfig{JuniorMaxComplexity: 3, IntermediateMaxComplexity: 5}
	role := RoleForComplexity(8, cfg)
	if role != RoleSenior {
		t.Errorf("complexity 8 should be Senior, got %s", role)
	}
}

func TestRoleForComplexity_Boundary(t *testing.T) {
	cfg := config.RoutingConfig{JuniorMaxComplexity: 3, IntermediateMaxComplexity: 5}
	// Exactly at junior boundary
	if RoleForComplexity(3, cfg) != RoleJunior {
		t.Error("complexity 3 should be Junior (<=)")
	}
	// Exactly at intermediate boundary
	if RoleForComplexity(5, cfg) != RoleIntermediate {
		t.Error("complexity 5 should be Intermediate (<=)")
	}
}

func TestModelConfig_ReturnsCorrectBinding(t *testing.T) {
	models := config.ModelsConfig{
		TechLead: config.ModelConfig{Provider: "anthropic", Model: "claude-opus-4-20250514"},
		Senior:   config.ModelConfig{Provider: "anthropic", Model: "claude-sonnet-4-20250514"},
		Junior:   config.ModelConfig{Provider: "openai", Model: "gpt-4o-mini"},
	}

	cfg := RoleSenior.ModelConfig(models)
	if cfg.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected sonnet for senior, got %s", cfg.Model)
	}

	cfg = RoleJunior.ModelConfig(models)
	if cfg.Provider != "openai" {
		t.Errorf("expected openai for junior, got %s", cfg.Provider)
	}
}

func TestModelConfig_AllRoles(t *testing.T) {
	models := config.ModelsConfig{
		TechLead:     config.ModelConfig{Provider: "anthropic", Model: "opus"},
		Senior:       config.ModelConfig{Provider: "anthropic", Model: "sonnet"},
		Intermediate: config.ModelConfig{Provider: "anthropic", Model: "sonnet-mid"},
		Junior:       config.ModelConfig{Provider: "openai", Model: "gpt-4o-mini"},
		QA:           config.ModelConfig{Provider: "anthropic", Model: "haiku"},
		Supervisor:   config.ModelConfig{Provider: "anthropic", Model: "opus-super"},
	}

	tests := []struct {
		role     Role
		expected string
	}{
		{RoleTechLead, "opus"},
		{RoleSenior, "sonnet"},
		{RoleIntermediate, "sonnet-mid"},
		{RoleJunior, "gpt-4o-mini"},
		{RoleQA, "haiku"},
		{RoleSupervisor, "opus-super"},
	}

	for _, tt := range tests {
		cfg := tt.role.ModelConfig(models)
		if cfg.Model != tt.expected {
			t.Errorf("role %s: expected model %s, got %s", tt.role, tt.expected, cfg.Model)
		}
	}
}

func TestModelConfig_UnknownRoleFallsBackToJunior(t *testing.T) {
	models := config.ModelsConfig{
		Junior: config.ModelConfig{Provider: "openai", Model: "gpt-4o-mini"},
	}

	unknown := Role("unknown")
	cfg := unknown.ModelConfig(models)
	if cfg.Model != "gpt-4o-mini" {
		t.Errorf("unknown role should fall back to junior model, got %s", cfg.Model)
	}
}

func TestAllRoles(t *testing.T) {
	roles := AllRoles()
	if len(roles) != 6 {
		t.Errorf("expected 6 roles, got %d", len(roles))
	}

	expected := map[Role]bool{
		RoleTechLead:     true,
		RoleSenior:       true,
		RoleIntermediate: true,
		RoleJunior:       true,
		RoleQA:           true,
		RoleSupervisor:   true,
	}

	for _, r := range roles {
		if !expected[r] {
			t.Errorf("unexpected role in AllRoles: %s", r)
		}
	}
}
