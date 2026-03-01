package terraform

import "testing"

func TestValidateReadOnlySubcommand(t *testing.T) {
	blocked := []string{"apply", "destroy", "import", "taint", "untaint", "force-unlock"}
	for _, cmd := range blocked {
		if err := ValidateReadOnlySubcommand(cmd); err == nil {
			t.Fatalf("expected %s to be blocked", cmd)
		}
	}
	allowed := []string{"init", "plan", "show", "fmt", "validate"}
	for _, cmd := range allowed {
		if err := ValidateReadOnlySubcommand(cmd); err != nil {
			t.Fatalf("expected %s to be allowed, got %v", cmd, err)
		}
	}
}
