package integration

import (
	"context"
	"os"
	"testing"

	"github.com/fluentorbit/restore-drill/pkg/engine"
	"github.com/fluentorbit/restore-drill/pkg/providers/mysql"
	"github.com/fluentorbit/restore-drill/pkg/providers/postgres"
	"github.com/fluentorbit/restore-drill/pkg/providers/redis"
	"github.com/fluentorbit/restore-drill/pkg/reporter"
	"github.com/fluentorbit/restore-drill/pkg/runtime/docker"
)

func TestIntegration(t *testing.T) {
	if os.Getenv("RESTORE_DRILL_INTEGRATION") == "" {
		t.Skip("set RESTORE_DRILL_INTEGRATION=1 to run integration tests")
	}

	cfg, err := engine.LoadConfig("drill.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	rt, err := docker.New()
	if err != nil {
		t.Fatalf("init docker: %v", err)
	}

	rep := reporter.NewStdout()
	eng := engine.New(rt, rep)
	eng.RegisterProvider(postgres.New())
	eng.RegisterProvider(mysql.New())
	eng.RegisterProvider(redis.New())

	results, err := eng.Run(context.Background(), cfg.Drills)
	if err != nil {
		t.Fatalf("engine run: %v", err)
	}

	for _, r := range results {
		t.Run(r.Name, func(t *testing.T) {
			if r.Error != nil {
				t.Errorf("drill error: %v", r.Error)
			}
			if !r.ValidationPassed {
				for _, c := range r.Checks {
					if !c.Passed {
						t.Errorf("check %q failed: expected %s, got %s", c.Name, c.Expected, c.Actual)
					}
				}
			}
		})
	}
}
