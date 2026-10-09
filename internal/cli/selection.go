package cli

import (
	"fmt"
	"strings"

	"github.com/RamazanKara/restore-drill/internal/config"
)

func selectDrills(drills []config.DrillConfig, names []string) ([]config.DrillConfig, error) {
	if len(names) == 0 {
		return drills, nil
	}
	available := make([]string, 0, len(drills))
	known := make(map[string]bool, len(drills))
	for _, drill := range drills {
		available = append(available, drill.Name)
		known[drill.Name] = true
	}
	selected := make(map[string]bool, len(names))
	for _, name := range names {
		if !known[name] {
			return nil, fmt.Errorf("unknown drill %q (available: %s)", name, strings.Join(available, ", "))
		}
		selected[name] = true
	}
	result := make([]config.DrillConfig, 0, len(selected))
	for _, drill := range drills {
		if selected[drill.Name] {
			result = append(result, drill)
		}
	}
	return result, nil
}
