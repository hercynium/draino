package kubernetes

import (
	"github.com/pkg/errors"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// A PodFilterFunc returns true if the supplied pod passes the filter.
type PodFilterFunc func(p core.Pod) (bool, error)

// MirrorPodFilter returns true if the supplied pod is not a mirror pod, i.e. a
// pod created by a manifest on the node rather than the API server.
func MirrorPodFilter(p core.Pod) (bool, error) {
	_, mirrorPod := p.GetAnnotations()[core.MirrorPodAnnotationKey]
	return !mirrorPod, nil
}

// LocalStoragePodFilter returns true if the supplied pod does not have local
// storage, i.e. does not use any 'empty dir' volumes.
func LocalStoragePodFilter(p core.Pod) (bool, error) {
	for _, v := range p.Spec.Volumes {
		if v.EmptyDir != nil {
			return false, nil
		}
	}
	return true, nil
}

// UnreplicatedPodFilter returns true if the pod is replicated, i.e. is managed
// by a controller (deployment, daemonset, statefulset, etc) of some sort.
func UnreplicatedPodFilter(p core.Pod) (bool, error) {
	// We're fine with 'evicting' unreplicated pods that aren't actually running.
	if p.Status.Phase == core.PodSucceeded || p.Status.Phase == core.PodFailed {
		return true, nil
	}
	if meta.GetControllerOf(&p) == nil {
		return false, nil
	}
	return true, nil
}

// NewDaemonSetPodFilter returns a FilterFunc that returns true if the supplied
// pod is not managed by an extant DaemonSet.
func NewDaemonSetPodFilter(client kubernetes.Interface) PodFilterFunc {
	return func(p core.Pod) (bool, error) {
		c := meta.GetControllerOf(&p)
		if c == nil || c.Kind != kindDaemonSet {
			return true, nil
		}

		// Pods pass the filter if they were created by a DaemonSet that no
		// longer exists.
		if _, err := client.ExtensionsV1beta1().DaemonSets(p.GetNamespace()).Get(c.Name, meta.GetOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, errors.Wrapf(err, "cannot get DaemonSet %s/%s", p.GetNamespace(), c.Name)
		}
		return false, nil
	}
}

// NewPodFilters returns a FilterFunc that returns true if all of the supplied
// FilterFuncs return true.
func NewPodFilters(filters ...PodFilterFunc) PodFilterFunc {
	return func(p core.Pod) (bool, error) {
		for _, fn := range filters {
			passes, err := fn(p)
			if err != nil {
				return false, errors.Wrap(err, "cannot apply filters")
			}
			if !passes {
				return false, nil
			}
		}
		return true, nil
	}
}
