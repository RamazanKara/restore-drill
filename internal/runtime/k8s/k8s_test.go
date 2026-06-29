package k8s

import (
	"context"
	"errors"
	"testing"

	"github.com/RamazanKara/restore-drill/internal/engine"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestDestroyUsesPodNamespace(t *testing.T) {
	ctx := context.Background()
	const (
		createdNamespace = "restore-drill-a"
		runtimeNamespace = "restore-drill-b"
		podName          = "drill-123"
	)

	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: createdNamespace,
		},
	})
	runtime := &Runtime{
		client:    client,
		namespace: runtimeNamespace,
	}

	err := runtime.Destroy(ctx, &pod{name: podName, namespace: createdNamespace})
	if err != nil {
		t.Fatalf("destroy pod: %v", err)
	}

	_, err = client.CoreV1().Pods(createdNamespace).Get(ctx, podName, metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected pod to be deleted from %s, got error %v", createdNamespace, err)
	}
}

func TestOptionsCopyInputs(t *testing.T) {
	labels := map[string]string{"team": "platform"}
	annotations := map[string]string{"reason": "nightly"}
	secrets := []string{"registry-creds"}
	runtime := &Runtime{}

	WithNamespace("restore-drill")(runtime)
	WithServiceAccountName("restore-drill-target")(runtime)
	WithPodLabels(labels)(runtime)
	WithPodAnnotations(annotations)(runtime)
	WithImagePullSecrets(secrets)(runtime)

	labels["team"] = "changed"
	annotations["reason"] = "changed"
	secrets[0] = "changed"

	if runtime.namespace != "restore-drill" {
		t.Fatalf("expected namespace to be set, got %q", runtime.namespace)
	}
	if runtime.serviceAccountName != "restore-drill-target" {
		t.Fatalf("expected service account to be set, got %q", runtime.serviceAccountName)
	}
	if runtime.podLabels["team"] != "platform" {
		t.Fatalf("expected labels to be copied, got %#v", runtime.podLabels)
	}
	if runtime.podAnnotations["reason"] != "nightly" {
		t.Fatalf("expected annotations to be copied, got %#v", runtime.podAnnotations)
	}
	if runtime.imagePullSecrets[0] != "registry-creds" {
		t.Fatalf("expected image pull secrets to be copied, got %#v", runtime.imagePullSecrets)
	}
}

func TestPodAccessors(t *testing.T) {
	p := &pod{
		name:      "drill-123",
		namespace: "restore-drill",
		podIP:     "10.0.0.12",
		ports:     map[int]int{5432: 5432},
	}

	if p.ID() != "restore-drill/drill-123" {
		t.Fatalf("unexpected pod id %q", p.ID())
	}
	if p.Host() != "10.0.0.12" {
		t.Fatalf("unexpected pod host %q", p.Host())
	}
	if p.Port(5432) != 5432 {
		t.Fatalf("unexpected pod port %d", p.Port(5432))
	}
}

func TestCreateReturnsRunningPodDetails(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	client.PrependReactor("get", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		get := action.(ktesting.GetAction)
		return true, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: get.GetName(), Namespace: "restore-drill"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				PodIP: "10.0.0.20",
			},
		}, nil
	})
	runtime := &Runtime{
		client:    client,
		namespace: "restore-drill",
	}

	container, err := runtime.Create(ctx, engine.ContainerSpec{
		Image: "postgres:16",
		Ports: []int{5432},
	})
	if err != nil {
		t.Fatalf("create pod: %v", err)
	}
	if container.ID() == "" || container.Host() != "10.0.0.20" || container.Port(5432) != 5432 {
		t.Fatalf("unexpected container details: id=%q host=%q port=%d", container.ID(), container.Host(), container.Port(5432))
	}
}

func TestBuildPodSpecAppliesRuntimePodOptions(t *testing.T) {
	runtime := &Runtime{
		namespace: "restore-drill",
		podLabels: map[string]string{
			"team":                    "platform",
			"restore-drill/ephemeral": "false",
		},
		podAnnotations: map[string]string{
			"backup.example.com/reason": "nightly-drill",
		},
		imagePullSecrets:   []string{"registry-creds"},
		serviceAccountName: "restore-drill-target",
	}

	podSpec, portMap, err := runtime.buildPodSpec("drill-123", engine.ContainerSpec{
		Image: "postgres:16",
		Ports: []int{5432},
		Env: map[string]string{
			"POSTGRES_HOST_AUTH_METHOD": "trust",
		},
		MemoryLimit: 512 * 1024 * 1024,
		CPULimit:    500_000_000,
	})
	if err != nil {
		t.Fatalf("build pod spec: %v", err)
	}

	if podSpec.Namespace != "restore-drill" {
		t.Fatalf("expected namespace restore-drill, got %q", podSpec.Namespace)
	}
	if podSpec.Labels["team"] != "platform" {
		t.Fatalf("expected custom label to be applied, got %#v", podSpec.Labels)
	}
	if podSpec.Labels["restore-drill/ephemeral"] != "true" {
		t.Fatalf("expected reserved ephemeral label to be enforced, got %#v", podSpec.Labels)
	}
	if podSpec.Annotations["backup.example.com/reason"] != "nightly-drill" {
		t.Fatalf("expected custom annotation to be applied, got %#v", podSpec.Annotations)
	}
	if podSpec.Spec.ServiceAccountName != "restore-drill-target" {
		t.Fatalf("expected target service account, got %q", podSpec.Spec.ServiceAccountName)
	}
	if len(podSpec.Spec.ImagePullSecrets) != 1 || podSpec.Spec.ImagePullSecrets[0].Name != "registry-creds" {
		t.Fatalf("expected image pull secret, got %#v", podSpec.Spec.ImagePullSecrets)
	}
	if got := portMap[5432]; got != 5432 {
		t.Fatalf("expected direct Kubernetes port mapping, got %d", got)
	}
	if got := podSpec.Spec.Containers[0].Resources.Limits.Cpu().MilliValue(); got != 500 {
		t.Fatalf("expected 500m CPU limit, got %dm", got)
	}
}

func TestBuildPodSpecSetsDefaultLabelsWithoutCustomOptions(t *testing.T) {
	runtime := &Runtime{namespace: "restore-drill"}

	podSpec, _, err := runtime.buildPodSpec("drill-123", engine.ContainerSpec{Image: "redis:7-alpine"})
	if err != nil {
		t.Fatalf("build pod spec: %v", err)
	}

	if podSpec.Labels["app.kubernetes.io/managed-by"] != "restore-drill" {
		t.Fatalf("expected managed-by label, got %#v", podSpec.Labels)
	}
	if podSpec.Labels["restore-drill/ephemeral"] != "true" {
		t.Fatalf("expected ephemeral label, got %#v", podSpec.Labels)
	}
}

func TestBuildPodSpecRejectsInvalidPorts(t *testing.T) {
	runtime := &Runtime{namespace: "restore-drill"}

	_, _, err := runtime.buildPodSpec("drill-123", engine.ContainerSpec{
		Image: "redis:7-alpine",
		Ports: []int{0},
	})
	if err == nil {
		t.Fatal("expected invalid port to fail")
	}
	if err.Error() != "invalid container port 0" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPingListsPodsInRuntimeNamespace(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "drill-123",
			Namespace: "restore-drill",
		},
	})
	runtime := &Runtime{
		client:    client,
		namespace: "restore-drill",
	}

	if err := runtime.Ping(ctx); err != nil {
		t.Fatalf("ping kubernetes runtime: %v", err)
	}
}

func TestPingReturnsListError(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	client.PrependReactor("list", "pods", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("api unavailable")
	})
	runtime := &Runtime{
		client:    client,
		namespace: "restore-drill",
	}

	err := runtime.Ping(ctx)
	if err == nil {
		t.Fatal("expected ping error")
	}
	if err.Error() != `list pods in namespace "restore-drill": api unavailable` {
		t.Fatalf("unexpected ping error: %v", err)
	}
}

func TestWaitPodReadyReturnsWhenRunning(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "drill-123", Namespace: "restore-drill"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	})
	runtime := &Runtime{
		client:    client,
		namespace: "restore-drill",
	}

	if err := runtime.waitPodReady(ctx, "drill-123"); err != nil {
		t.Fatalf("wait pod ready: %v", err)
	}
}

func TestWaitPodReadyReturnsTerminalPhase(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "drill-123", Namespace: "restore-drill"},
		Status:     corev1.PodStatus{Phase: corev1.PodFailed},
	})
	runtime := &Runtime{
		client:    client,
		namespace: "restore-drill",
	}

	err := runtime.waitPodReady(ctx, "drill-123")
	if err == nil {
		t.Fatal("expected terminal phase error")
	}
	if err.Error() != "pod drill-123 in terminal phase: Failed" {
		t.Fatalf("unexpected wait error: %v", err)
	}
}
