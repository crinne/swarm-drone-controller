package kube

import (
	"context"
	"errors"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/crinne/swarm-drone-controller/internal/spawn"
)

func TestListDroneIDsReadsLabels(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "drone-1",
				Namespace: "swarm",
				Labels: map[string]string{
					"app.kubernetes.io/component": "drone",
					"swarmgcs.dev/drone-id":       "1",
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "not-a-drone",
				Namespace: "swarm",
				Labels: map[string]string{
					"app.kubernetes.io/component": "controller",
					"swarmgcs.dev/drone-id":       "99",
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "drone-4",
				Namespace: "swarm",
				Labels: map[string]string{
					"app.kubernetes.io/component": "drone",
					"swarmgcs.dev/drone-id":       "4",
				},
			},
		},
	)
	store := NewStore(client, "swarm", "ghcr.io/crinne/swarm-sim:test")

	ids, err := store.ListDroneIDs(context.Background())
	if err != nil {
		t.Fatalf("ListDroneIDs returned error: %v", err)
	}
	if !reflect.DeepEqual(ids, []int{1, 4}) {
		t.Fatalf("ids = %v, want [1 4]", ids)
	}
}

func TestListDroneIDsDeletesTerminalDronePods(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "drone-4",
				Namespace: "swarm",
				Labels: map[string]string{
					"app.kubernetes.io/component": "drone",
					"swarmgcs.dev/drone-id":       "4",
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "drone-5",
				Namespace: "swarm",
				Labels: map[string]string{
					"app.kubernetes.io/component": "drone",
					"swarmgcs.dev/drone-id":       "5",
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)
	store := NewStore(client, "swarm", "ghcr.io/crinne/swarm-sim:test")

	ids, err := store.ListDroneIDs(context.Background())
	if err != nil {
		t.Fatalf("ListDroneIDs returned error: %v", err)
	}
	if !reflect.DeepEqual(ids, []int{5}) {
		t.Fatalf("ids = %v, want [5]", ids)
	}
	if _, err := client.CoreV1().Pods("swarm").Get(context.Background(), "drone-4", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("terminal drone pod error = %v, want not found", err)
	}
}

func TestCreateDroneBuildsExpectedPod(t *testing.T) {
	client := fake.NewSimpleClientset()
	store := NewStore(client, "swarm", "ghcr.io/crinne/swarm-sim:test")

	err := store.CreateDrone(context.Background(), 4)
	if err != nil {
		t.Fatalf("CreateDrone returned error: %v", err)
	}

	pod, err := client.CoreV1().Pods("swarm").Get(context.Background(), "drone-4", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get created pod: %v", err)
	}

	wantLabels := map[string]string{
		"app.kubernetes.io/name":      "swarm-sim",
		"app.kubernetes.io/component": "drone",
		"swarmgcs.dev/drone-id":       "4",
	}
	if !reflect.DeepEqual(pod.Labels, wantLabels) {
		t.Fatalf("labels = %v, want %v", pod.Labels, wantLabels)
	}
	if pod.Spec.RestartPolicy != corev1.RestartPolicyOnFailure {
		t.Fatalf("restartPolicy = %q, want %q", pod.Spec.RestartPolicy, corev1.RestartPolicyOnFailure)
	}
	if !reflect.DeepEqual(pod.Spec.ImagePullSecrets, []corev1.LocalObjectReference{{Name: "ghcr-pull"}}) {
		t.Fatalf("imagePullSecrets = %v, want ghcr-pull", pod.Spec.ImagePullSecrets)
	}
	if len(pod.Spec.Containers) != 1 {
		t.Fatalf("containers length = %d, want 1", len(pod.Spec.Containers))
	}

	container := pod.Spec.Containers[0]
	if container.Name != "drone" {
		t.Fatalf("container name = %q, want drone", container.Name)
	}
	if container.Image != "ghcr.io/crinne/swarm-sim:test" {
		t.Fatalf("image = %q, want configured image", container.Image)
	}
	if !reflect.DeepEqual(container.Command, []string{"/usr/local/bin/drone"}) {
		t.Fatalf("command = %v, want drone binary", container.Command)
	}
	wantArgs := []string{"--id", "4", "--proxy", "swarm-proxy", "--port", "14550"}
	if !reflect.DeepEqual(container.Args, wantArgs) {
		t.Fatalf("args = %v, want %v", container.Args, wantArgs)
	}
	if container.Resources.Requests.Cpu().String() != "10m" {
		t.Fatalf("cpu request = %s, want 10m", container.Resources.Requests.Cpu().String())
	}
	if container.Resources.Requests.Memory().String() != "16Mi" {
		t.Fatalf("memory request = %s, want 16Mi", container.Resources.Requests.Memory().String())
	}
	if container.Resources.Limits.Cpu().String() != "100m" {
		t.Fatalf("cpu limit = %s, want 100m", container.Resources.Limits.Cpu().String())
	}
	if container.Resources.Limits.Memory().String() != "64Mi" {
		t.Fatalf("memory limit = %s, want 64Mi", container.Resources.Limits.Memory().String())
	}
	if container.SecurityContext == nil {
		t.Fatal("securityContext is nil")
	}
	if container.SecurityContext.AllowPrivilegeEscalation == nil || *container.SecurityContext.AllowPrivilegeEscalation {
		t.Fatalf("allowPrivilegeEscalation = %v, want false", container.SecurityContext.AllowPrivilegeEscalation)
	}
	if container.SecurityContext.RunAsNonRoot == nil || !*container.SecurityContext.RunAsNonRoot {
		t.Fatalf("runAsNonRoot = %v, want true", container.SecurityContext.RunAsNonRoot)
	}
}

func TestCreateDroneMapsAlreadyExists(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "drone-4",
			Namespace: "swarm",
		},
	})
	store := NewStore(client, "swarm", "ghcr.io/crinne/swarm-sim:test")

	err := store.CreateDrone(context.Background(), 4)
	if !errors.Is(err, spawn.ErrAlreadyExists) {
		t.Fatalf("error = %v, want ErrAlreadyExists", err)
	}
}
