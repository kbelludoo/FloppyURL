package skills

import (
	"context"
	"testing"
)

func TestSkillRegistry(t *testing.T) {
	reg := NewRegistry()

	skill := &Skill{
		ID:          "test-skill",
		Name:        "Test Capability",
		Description: "A skill for testing",
		Prompt:      "Execute tests carefully.",
		Tools: []SkillTool{
			{
				Name:        "test_ping",
				Description: "Pings the test harness",
				Execute: func(ctx context.Context, args string) (string, error) {
					return "pong: " + args, nil
				},
			},
		},
	}

	reg.Register(skill)

	list := reg.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(list))
	}

	if reg.IsActive("test-skill") {
		t.Fatalf("skill should be inactive by default")
	}

	// Activate
	if _, err := reg.Activate("test-skill"); err != nil {
		t.Fatalf("activation failed: %v", err)
	}

	if !reg.IsActive("test-skill") {
		t.Fatalf("skill should now be active")
	}

	prompt := reg.RenderActivePrompts()
	if prompt == "" {
		t.Fatalf("expected active prompt to be non-empty")
	}

	// Tool execution
	res, handled, err := reg.ExecuteActiveTool(context.Background(), "test_ping", `{"foo":"bar"}`)
	if !handled || err != nil {
		t.Fatalf("expected handled tool execution, got handled=%v, err=%v", handled, err)
	}
	if res != `pong: {"foo":"bar"}` {
		t.Fatalf("unexpected result: %q", res)
	}

	// Deactivate
	reg.Deactivate("test-skill")
	if reg.IsActive("test-skill") {
		t.Fatalf("skill should be inactive after deactivation")
	}

	_, handledAfter, _ := reg.ExecuteActiveTool(context.Background(), "test_ping", "")
	if handledAfter {
		t.Fatalf("tool should not be handled when skill is deactivated")
	}
}
