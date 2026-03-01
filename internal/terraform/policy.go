package terraform

import (
	"fmt"
	"strings"
)

var blockedSubcommands = map[string]struct{}{
	"apply":        {},
	"destroy":      {},
	"import":       {},
	"taint":        {},
	"untaint":      {},
	"force-unlock": {},
}

func ValidateReadOnlySubcommand(subcommand string) error {
	cmd := strings.ToLower(strings.TrimSpace(subcommand))
	if _, blocked := blockedSubcommands[cmd]; blocked {
		return fmt.Errorf("terraform command %q is blocked by read-only policy", cmd)
	}
	return nil
}
