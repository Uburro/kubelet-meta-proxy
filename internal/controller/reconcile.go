package controller

import (
	"context"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nsmetrics "github.com/Uburro/kubelet-meta-proxy/internal/metrics"
)

// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=node/proxy,verbs=get;list;watch

// NamespaceLabelReconciler reconciles a Namespace object.
type NamespaceLabelReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	NamespaceMetrics *nsmetrics.NamespaceMetrics
}

// Reconcile reads that state of the cluster for a Namespace object and add labels to NamespaceMetrics map.
func (r *NamespaceLabelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("NamespaceLabelReconciler")
	logger.Info("Reconciling Namespace", "namespace", req.NamespacedName)

	ns := &corev1.Namespace{}
	if err := r.Get(ctx, req.NamespacedName, ns); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	labels := ns.GetLabels()
	if len(labels) == 0 {
		return ctrl.Result{}, nil
	}

	for label := range labels {
		if label == corev1.LabelMetadataName {
			delete(labels, label)
		}
	}

	r.NamespaceMetrics.Namespaces[ns.Name] = labels
	logger.Info("Namespace labels added to NamespaceMetrics", "namespace", ns.Name, "labels", labels)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceLabelReconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrency int, cacheSyncTimeout time.Duration) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		WithOptions(controllerOptions(maxConcurrency, cacheSyncTimeout)).
		Complete(r)
}

var (
	optionsInit    sync.Once
	defaultOptions *controller.Options
)

// ControllerOptions is rate limiters and cache sync timeout for the controller.
func controllerOptions(maxConcurrency int, cacheSyncTimeout time.Duration) controller.Options {
	rateLimiters := workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](30*time.Second, 5*time.Minute)
	optionsInit.Do(func() {
		defaultOptions = &controller.Options{
			RateLimiter:             rateLimiters,
			CacheSyncTimeout:        cacheSyncTimeout,
			MaxConcurrentReconciles: maxConcurrency,
		}
	})
	return *defaultOptions
}
