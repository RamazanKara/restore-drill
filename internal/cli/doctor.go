package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/RamazanKara/restore-drill/internal/config"
	"github.com/RamazanKara/restore-drill/internal/runtime/docker"
	"github.com/RamazanKara/restore-drill/internal/runtime/k8s"
	"github.com/RamazanKara/restore-drill/internal/state"
	"github.com/spf13/cobra"
)

type doctorOptions struct {
	configPath string
	runtime    string
	format     string
	strict     bool
}

type doctorStatus string

const (
	doctorPass doctorStatus = "pass"
	doctorWarn doctorStatus = "warn"
	doctorFail doctorStatus = "fail"
)

type doctorCheck struct {
	Name        string       `json:"name"`
	Status      doctorStatus `json:"status"`
	Detail      string       `json:"detail"`
	Remediation string       `json:"remediation,omitempty"`
}

type doctorSummary struct {
	Passed   int `json:"passed"`
	Warnings int `json:"warnings"`
	Failed   int `json:"failed"`
}

type doctorReport struct {
	Status  doctorStatus  `json:"status"`
	Strict  bool          `json:"strict"`
	Summary doctorSummary `json:"summary"`
	Checks  []doctorCheck `json:"checks"`
}

type doctorDeps struct {
	loadConfig      func(string) (*config.Config, error)
	dockerPing      func(context.Context) error
	kubernetesPing  func(context.Context, string) error
	lookPath        func(string) (string, error)
	writableDir     func(string) error
	isKubernetesEnv func() bool
}

var doctorDepsFactory = defaultDoctorDeps

func doctorCmd() *cobra.Command {
	opts := doctorOptions{}
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check local restore-drill configuration, runtime, and release tooling",
		RunE: func(cmd *cobra.Command, args []string) error {
			report := runDoctor(cmd.Context(), opts, doctorDepsFactory())
			if err := renderDoctorReport(cmd.OutOrStdout(), opts.format, report); err != nil {
				return err
			}
			if report.Summary.Failed > 0 {
				return fmt.Errorf("%d doctor check(s) failed", report.Summary.Failed)
			}
			if opts.strict && report.Summary.Warnings > 0 {
				return fmt.Errorf("%d doctor warning(s) in strict mode", report.Summary.Warnings)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&opts.configPath, "config", "c", "drill.yaml", "Path to drill configuration file")
	cmd.Flags().StringVar(&opts.runtime, "runtime", "auto", "Runtime to check: auto, docker, kubernetes")
	cmd.Flags().StringVar(&opts.format, "format", "table", "Output format: table, json")
	cmd.Flags().BoolVar(&opts.strict, "strict", false, "Treat warnings as failures")
	return cmd
}

func defaultDoctorDeps() doctorDeps {
	return doctorDeps{
		loadConfig: config.LoadConfig,
		dockerPing: func(ctx context.Context) error {
			rt, err := docker.New()
			if err != nil {
				return err
			}
			return rt.Ping(ctx)
		},
		kubernetesPing: func(ctx context.Context, namespace string) error {
			rt, err := k8s.New(k8s.WithNamespace(namespace))
			if err != nil {
				return err
			}
			return rt.Ping(ctx)
		},
		lookPath:        exec.LookPath,
		writableDir:     checkWritableDir,
		isKubernetesEnv: func() bool { return os.Getenv("KUBERNETES_SERVICE_HOST") != "" },
	}
}

func runDoctor(ctx context.Context, opts doctorOptions, deps doctorDeps) doctorReport {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	report := doctorReport{Strict: opts.strict}

	var cfg *config.Config
	if loaded, err := deps.loadConfig(opts.configPath); err != nil {
		report.add(doctorCheck{
			Name:        "config",
			Status:      doctorFail,
			Detail:      err.Error(),
			Remediation: "Pass --config with a valid drill YAML file or run restore-drill validate first.",
		})
	} else {
		cfg = loaded
		report.add(doctorCheck{
			Name:   "config",
			Status: doctorPass,
			Detail: fmt.Sprintf("%s is valid with %d drill(s)", opts.configPath, len(cfg.Drills)),
		})
	}

	runtimeMode := strings.TrimSpace(opts.runtime)
	switch runtimeMode {
	case "auto", "docker", "kubernetes":
		report.add(doctorCheck{Name: "runtime selection", Status: doctorPass, Detail: "runtime mode " + runtimeMode + " is supported"})
	default:
		report.add(doctorCheck{
			Name:        "runtime selection",
			Status:      doctorFail,
			Detail:      fmt.Sprintf("unknown runtime %q", runtimeMode),
			Remediation: "Use --runtime auto, --runtime docker, or --runtime kubernetes.",
		})
	}

	if runtimeMode == "auto" {
		if deps.isKubernetesEnv() {
			runtimeMode = "kubernetes"
		} else {
			runtimeMode = "docker"
		}
		report.add(doctorCheck{Name: "runtime auto", Status: doctorPass, Detail: "auto resolves to " + runtimeMode})
	}

	switch runtimeMode {
	case "docker":
		if err := deps.dockerPing(ctx); err != nil {
			report.add(doctorCheck{
				Name:        "docker daemon",
				Status:      doctorFail,
				Detail:      err.Error(),
				Remediation: "Start Docker or use --runtime kubernetes when running in a cluster.",
			})
		} else {
			report.add(doctorCheck{Name: "docker daemon", Status: doctorPass, Detail: "Docker daemon is reachable"})
		}
	case "kubernetes":
		namespace := "restore-drill"
		if err := deps.kubernetesPing(ctx, namespace); err != nil {
			report.add(doctorCheck{
				Name:        "kubernetes api",
				Status:      doctorFail,
				Detail:      err.Error(),
				Remediation: "Check kubeconfig, in-cluster credentials, namespace RBAC, or use --runtime docker locally.",
			})
		} else {
			report.add(doctorCheck{Name: "kubernetes api", Status: doctorPass, Detail: "Kubernetes API is reachable"})
		}
	}

	stateDir := filepath.Dir(state.DefaultPath())
	if err := deps.writableDir(stateDir); err != nil {
		report.add(doctorCheck{
			Name:        "state directory",
			Status:      doctorFail,
			Detail:      err.Error(),
			Remediation: "Ensure the restore-drill state directory is writable by the current user.",
		})
	} else {
		report.add(doctorCheck{Name: "state directory", Status: doctorPass, Detail: stateDir + " is writable"})
	}

	if cfg == nil || strings.TrimSpace(cfg.Reporting.Output) == "" {
		report.add(doctorCheck{
			Name:        "report output",
			Status:      doctorWarn,
			Detail:      "reporting.output is not configured",
			Remediation: "Set reporting.output when scheduled drills should leave durable JSON or HTML evidence files.",
		})
	} else {
		path := configuredReportPath(cfg.Reporting.Output, "json", false, time.Now().UTC().Format("20060102T150405Z"))
		dir := filepath.Dir(path)
		if err := deps.writableDir(dir); err != nil {
			report.add(doctorCheck{
				Name:        "report output",
				Status:      doctorFail,
				Detail:      err.Error(),
				Remediation: "Ensure reporting.output points to a writable directory or file path.",
			})
		} else {
			report.add(doctorCheck{Name: "report output", Status: doctorPass, Detail: dir + " is writable"})
		}
	}

	addToolChecks(&report, deps)
	report.finalize()
	return report
}

func addToolChecks(report *doctorReport, deps doctorDeps) {
	required := []string{"go", "git"}
	optional := []string{"docker", "helm", "goreleaser", "syft", "kind", "kubectl", "golangci-lint", "cosign", "govulncheck"}

	for _, name := range required {
		if path, err := deps.lookPath(name); err != nil {
			report.add(doctorCheck{
				Name:        "tool:" + name,
				Status:      doctorFail,
				Detail:      name + " not found on PATH",
				Remediation: "Install " + name + " before running local development and release checks.",
			})
		} else {
			report.add(doctorCheck{Name: "tool:" + name, Status: doctorPass, Detail: path})
		}
	}

	for _, name := range optional {
		if path, err := deps.lookPath(name); err != nil {
			report.add(doctorCheck{
				Name:        "tool:" + name,
				Status:      doctorWarn,
				Detail:      name + " not found on PATH",
				Remediation: optionalToolRemediation(name),
			})
		} else {
			report.add(doctorCheck{Name: "tool:" + name, Status: doctorPass, Detail: path})
		}
	}
}

func optionalToolRemediation(name string) string {
	switch name {
	case "govulncheck":
		return "Install govulncheck for offline scans; make vuln can still use go run when network access is available."
	case "cosign":
		return "Install cosign before verifying or signing release artifacts locally."
	default:
		return "Install " + name + " before running the full local release gate."
	}
}

func (r *doctorReport) add(check doctorCheck) {
	r.Checks = append(r.Checks, check)
}

func (r *doctorReport) finalize() {
	r.Status = doctorPass
	for _, check := range r.Checks {
		switch check.Status {
		case doctorFail:
			r.Summary.Failed++
			r.Status = doctorFail
		case doctorWarn:
			r.Summary.Warnings++
			if r.Status == doctorPass {
				r.Status = doctorWarn
			}
		default:
			r.Summary.Passed++
		}
	}
	if r.Strict && r.Summary.Warnings > 0 && r.Status != doctorFail {
		r.Status = doctorFail
	}
}

func renderDoctorReport(w io.Writer, format string, report doctorReport) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	case "table":
		return renderDoctorTable(w, report)
	default:
		return fmt.Errorf("unknown doctor output format %q (use table or json)", format)
	}
}

func renderDoctorTable(w io.Writer, report doctorReport) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "STATUS\tCHECK\tDETAIL"); err != nil {
		return err
	}
	for _, check := range report.Checks {
		detail := check.Detail
		if check.Remediation != "" {
			detail += " (" + check.Remediation + ")"
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", strings.ToUpper(string(check.Status)), check.Name, detail); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(tw, "\nSUMMARY\t%s\t%d passed, %d warning(s), %d failed\n",
		strings.ToUpper(string(report.Status)),
		report.Summary.Passed,
		report.Summary.Warnings,
		report.Summary.Failed,
	); err != nil {
		return err
	}
	return tw.Flush()
}

func checkWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	f, err := os.CreateTemp(dir, ".restore-drill-doctor-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file in %s: %w", dir, err)
	}
	path := f.Name()
	closeErr := f.Close()
	removeErr := os.Remove(path)
	if closeErr != nil {
		return fmt.Errorf("close temporary file in %s: %w", dir, closeErr)
	}
	if removeErr != nil {
		return fmt.Errorf("remove temporary file in %s: %w", dir, removeErr)
	}
	return nil
}
