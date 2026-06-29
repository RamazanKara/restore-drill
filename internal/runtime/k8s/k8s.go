package k8s

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/RamazanKara/restore-drill/internal/engine"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

// Runtime implements engine.Runtime using Kubernetes pods.
type Runtime struct {
	client             kubernetes.Interface
	config             *rest.Config
	namespace          string
	podLabels          map[string]string
	podAnnotations     map[string]string
	imagePullSecrets   []string
	serviceAccountName string
}

// Option configures the Kubernetes runtime.
type Option func(*Runtime)

// WithNamespace sets the namespace for ephemeral pods.
func WithNamespace(ns string) Option {
	return func(r *Runtime) { r.namespace = ns }
}

// WithPodLabels adds labels to ephemeral restore pods.
func WithPodLabels(labels map[string]string) Option {
	return func(r *Runtime) { r.podLabels = copyStringMap(labels) }
}

// WithPodAnnotations adds annotations to ephemeral restore pods.
func WithPodAnnotations(annotations map[string]string) Option {
	return func(r *Runtime) { r.podAnnotations = copyStringMap(annotations) }
}

// WithImagePullSecrets sets image pull secrets for ephemeral restore pods.
func WithImagePullSecrets(names []string) Option {
	return func(r *Runtime) { r.imagePullSecrets = append([]string(nil), names...) }
}

// WithServiceAccountName sets the service account for ephemeral restore pods.
func WithServiceAccountName(name string) Option {
	return func(r *Runtime) { r.serviceAccountName = name }
}

// New creates a Kubernetes runtime, using in-cluster config or kubeconfig.
func New(opts ...Option) (*Runtime, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig
		config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(),
			&clientcmd.ConfigOverrides{},
		).ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("k8s config: %w", err)
		}
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("k8s client: %w", err)
	}

	r := &Runtime{
		client:    client,
		config:    config,
		namespace: "restore-drill",
	}
	for _, o := range opts {
		o(r)
	}
	return r, nil
}

// Ping verifies that the Kubernetes API is reachable with the configured namespace.
func (r *Runtime) Ping(ctx context.Context) error {
	if _, err := r.client.CoreV1().Pods(r.namespace).List(ctx, metav1.ListOptions{Limit: 1}); err != nil {
		return fmt.Errorf("list pods in namespace %q: %w", r.namespace, err)
	}
	return nil
}

// pod wraps a Kubernetes pod as an engine.Container.
type pod struct {
	name      string
	namespace string
	podIP     string
	ports     map[int]int
}

func (p *pod) ID() string             { return p.namespace + "/" + p.name }
func (p *pod) Host() string           { return p.podIP }
func (p *pod) Port(container int) int { return container } // In-cluster, ports are direct.

// Create provisions a pod with the given spec and waits for it to be ready.
func (r *Runtime) Create(ctx context.Context, spec engine.ContainerSpec) (engine.Container, error) {
	name := fmt.Sprintf("drill-%d", time.Now().UnixNano())
	podSpec, portMap, err := r.buildPodSpec(name, spec)
	if err != nil {
		return nil, err
	}

	created, err := r.client.CoreV1().Pods(r.namespace).Create(ctx, podSpec, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create pod: %w", err)
	}

	slog.Info("pod created", "name", created.Name, "namespace", r.namespace)

	// Wait for pod to be running.
	if err := r.waitPodReady(ctx, name); err != nil {
		// Attempt cleanup on failure.
		_ = r.client.CoreV1().Pods(r.namespace).Delete(ctx, name, metav1.DeleteOptions{})
		return nil, err
	}

	// Re-fetch pod for IP.
	running, err := r.client.CoreV1().Pods(r.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pod: %w", err)
	}

	p := &pod{
		name:      name,
		namespace: r.namespace,
		podIP:     running.Status.PodIP,
		ports:     portMap,
	}

	return p, nil
}

func (r *Runtime) buildPodSpec(name string, spec engine.ContainerSpec) (*corev1.Pod, map[int]int, error) {
	envVars := make([]corev1.EnvVar, 0, len(spec.Env))
	for k, v := range spec.Env {
		envVars = append(envVars, corev1.EnvVar{Name: k, Value: v})
	}

	containerPorts := make([]corev1.ContainerPort, 0, len(spec.Ports))
	portMap := make(map[int]int, len(spec.Ports))
	for _, p := range spec.Ports {
		if p <= 0 || p > 65535 {
			return nil, nil, fmt.Errorf("invalid container port %d", p)
		}
		containerPorts = append(containerPorts, corev1.ContainerPort{ContainerPort: int32(p)})
		portMap[p] = p
	}

	resources := corev1.ResourceRequirements{}
	if spec.MemoryLimit > 0 {
		resources.Limits = corev1.ResourceList{
			corev1.ResourceMemory: *resource.NewQuantity(spec.MemoryLimit, resource.BinarySI),
		}
	}
	if spec.CPULimit > 0 {
		if resources.Limits == nil {
			resources.Limits = corev1.ResourceList{}
		}
		resources.Limits[corev1.ResourceCPU] = *resource.NewMilliQuantity(spec.CPULimit/1_000_000, resource.DecimalSI)
	}

	labels := copyStringMap(r.podLabels)
	if labels == nil {
		labels = make(map[string]string, 2)
	}
	labels["app.kubernetes.io/managed-by"] = "restore-drill"
	labels["restore-drill/ephemeral"] = "true"

	imagePullSecrets := make([]corev1.LocalObjectReference, 0, len(r.imagePullSecrets))
	for _, name := range r.imagePullSecrets {
		if name == "" {
			continue
		}
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: name})
	}

	podSpec := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   r.namespace,
			Labels:      labels,
			Annotations: copyStringMap(r.podAnnotations),
		},
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			ImagePullSecrets:   imagePullSecrets,
			ServiceAccountName: r.serviceAccountName,
			Containers: []corev1.Container{
				{
					Name:      "db",
					Image:     spec.Image,
					Env:       envVars,
					Ports:     containerPorts,
					Resources: resources,
					Command:   spec.Cmd,
				},
			},
		},
	}

	return podSpec, portMap, nil
}

// Exec runs a command in the pod's first container.
func (r *Runtime) Exec(ctx context.Context, c engine.Container, cmd []string) ([]byte, error) {
	p := c.(*pod)

	req := r.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(p.name).
		Namespace(p.namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "db",
			Command:   cmd,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(r.config, "POST", req.URL())
	if err != nil {
		return nil, fmt.Errorf("exec setup: %w", err)
	}

	var stdout, stderr bytes.Buffer
	if err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	}); err != nil {
		return nil, fmt.Errorf("exec %v: %w (stderr: %s)", cmd, err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// CopyTo copies data into the pod via exec tar.
func (r *Runtime) CopyTo(ctx context.Context, c engine.Container, dest string, src io.Reader) error {
	p := c.(*pod)

	cmd := []string{"tar", "xf", "-", "-C", dest}
	req := r.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(p.name).
		Namespace(p.namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "db",
			Command:   cmd,
			Stdin:     true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(r.config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("copy setup: %w", err)
	}

	var stderr bytes.Buffer
	if err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  src,
		Stderr: &stderr,
	}); err != nil {
		return fmt.Errorf("copy to %s: %w (stderr: %s)", dest, err, stderr.String())
	}

	return nil
}

// Destroy deletes the pod.
func (r *Runtime) Destroy(ctx context.Context, c engine.Container) error {
	p := c.(*pod)
	err := r.client.CoreV1().Pods(p.namespace).Delete(ctx, p.name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("delete pod %s/%s: %w", p.namespace, p.name, err)
	}
	slog.Info("pod destroyed", "name", p.name, "namespace", p.namespace)
	return nil
}

// waitPodReady polls until the pod is in Running phase.
func (r *Runtime) waitPodReady(ctx context.Context, name string) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		p, err := r.client.CoreV1().Pods(r.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		switch p.Status.Phase {
		case corev1.PodRunning:
			return true, nil
		case corev1.PodFailed, corev1.PodSucceeded:
			return false, fmt.Errorf("pod %s in terminal phase: %s", name, p.Status.Phase)
		}
		return false, nil
	})
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
