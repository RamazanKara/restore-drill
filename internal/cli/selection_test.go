package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/RamazanKara/restore-drill/internal/config"
)

func TestSelectDrills(t *testing.T) {
	drills := []config.DrillConfig{{Name: "postgres"}, {Name: "redis"}, {Name: "name,with,commas"}}
	tests := []struct {
		name    string
		names   []string
		want    []config.DrillConfig
		wantErr string
	}{
		{name: "all", want: drills},
		{name: "one", names: []string{"redis"}, want: drills[1:2]},
		{name: "config order", names: []string{"redis", "postgres"}, want: drills[:2]},
		{name: "duplicates", names: []string{"redis", "redis"}, want: drills[1:2]},
		{name: "literal commas", names: []string{"name,with,commas"}, want: drills[2:]},
		{name: "unknown", names: []string{"redis", "missing"}, wantErr: `unknown drill "missing" (available: postgres, redis, name,with,commas)`},
		{name: "case sensitive", names: []string{"Redis"}, wantErr: `unknown drill "Redis"`},
		{name: "empty name", names: []string{""}, wantErr: `unknown drill ""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectDrills(drills, tt.names)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || got != nil {
					t.Fatalf("selectDrills() = %v, %v; want %q", got, err, tt.wantErr)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("selectDrills() = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}
