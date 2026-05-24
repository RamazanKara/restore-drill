package k8s

import (
	"context"
	"testing"

	"github.com/RamazanKara/restore-drill/pkg/engine"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
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
