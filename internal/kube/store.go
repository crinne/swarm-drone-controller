package kube

import (
	"context"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/utils/ptr"

	"github.com/crinne/swarm-drone-controller/internal/spawn"
)

const (
	droneComponentSelector = "app.kubernetes.io/component=drone"
	droneIDLabel           = "swarmgcs.dev/drone-id"
)

type Store struct {
	pods  corev1client.PodInterface
	image string
}

func NewStore(client kubernetes.Interface, namespace, image string) *Store {
	return &Store{
		pods:  client.CoreV1().Pods(namespace),
		image: image,
	}
}

func (s *Store) ListDroneIDs(ctx context.Context) ([]int, error) {
	list, err := s.pods.List(ctx, metav1.ListOptions{
		LabelSelector: droneComponentSelector,
	})
	if err != nil {
		return nil, err
	}

	ids := make([]int, 0, len(list.Items))
	for _, pod := range list.Items {
		if isTerminalPod(pod.Status.Phase) {
			if err := s.pods.Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				return nil, err
			}
			continue
		}
		rawID, ok := pod.Labels[droneIDLabel]
		if !ok {
			continue
		}
		id, err := strconv.Atoi(rawID)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}

	sort.Ints(ids)
	return ids, nil
}

func (s *Store) CreateDrone(ctx context.Context, id int) error {
	idString := strconv.Itoa(id)
	_, err := s.pods.Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "drone-" + idString,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "swarm-sim",
				"app.kubernetes.io/component": "drone",
				droneIDLabel:                  idString,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyOnFailure,
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: "ghcr-pull"},
			},
			Containers: []corev1.Container{
				{
					Name:    "drone",
					Image:   s.image,
					Command: []string{"/usr/local/bin/drone"},
					Args: []string{
						"--id", idString,
						"--proxy", "swarm-proxy",
						"--port", "14550",
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("10m"),
							corev1.ResourceMemory: resource.MustParse("16Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: ptr.To(false),
						RunAsNonRoot:             ptr.To(true),
					},
				},
			},
		},
	}, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return spawn.ErrAlreadyExists
	}
	return err
}

func isTerminalPod(phase corev1.PodPhase) bool {
	return phase == corev1.PodSucceeded || phase == corev1.PodFailed
}
