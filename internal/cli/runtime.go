package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/RamazanKara/restore-drill/internal/engine"
	"github.com/RamazanKara/restore-drill/internal/runtime/docker"
	"github.com/RamazanKara/restore-drill/internal/runtime/k8s"
)

type runtimeOptions struct {
	mode             string
	namespace        string
	serviceAccount   string
	podLabels        map[string]string
	podAnnotations   map[string]string
	imagePullSecrets []string
}

func newRuntime(opts runtimeOptions) (engine.Runtime, error) {
	kubeOptions := []k8s.Option{
		k8s.WithNamespace(opts.namespace),
		k8s.WithServiceAccountName(opts.serviceAccount),
		k8s.WithPodLabels(opts.podLabels),
		k8s.WithPodAnnotations(opts.podAnnotations),
		k8s.WithImagePullSecrets(opts.imagePullSecrets),
	}

	switch opts.mode {
	case "auto":
		if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
			rt, err := k8s.New(kubeOptions...)
			if err != nil {
				return nil, fmt.Errorf("init kubernetes runtime: %w", err)
			}
			return rt, nil
		}
		rt, err := docker.New()
		if err != nil {
			return nil, fmt.Errorf("init docker runtime: %w", err)
		}
		return rt, nil
	case "docker":
		rt, err := docker.New()
		if err != nil {
			return nil, fmt.Errorf("init docker runtime: %w", err)
		}
		return rt, nil
	case "kubernetes":
		rt, err := k8s.New(kubeOptions...)
		if err != nil {
			return nil, fmt.Errorf("init kubernetes runtime: %w", err)
		}
		return rt, nil
	default:
		return nil, fmt.Errorf("unknown runtime %q (use auto, docker, or kubernetes)", opts.mode)
	}
}

func parseKeyValueFlags(values []string, flagName string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	parsed := make(map[string]string, len(values))
	for _, value := range values {
		key, val, ok := strings.Cut(value, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("%s must use key=value syntax (got %q)", flagName, value)
		}
		parsed[key] = strings.TrimSpace(val)
	}
	return parsed, nil
}
