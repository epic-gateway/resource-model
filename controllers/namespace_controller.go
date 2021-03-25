package controllers

import (
	"context"
	"strings"

	marin3r "github.com/3scale/marin3r/apis/operator/v1alpha1"
	"github.com/go-logr/logr"
	egwv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	orgLabels = map[string]string{"app.kubernetes.io/component": "organization", "app.kubernetes.io/part-of": "epic"}
)

// NamespaceReconciler reconciles a Namespace object
type NamespaceReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups="",resources=namespaces,verbs=list;get;watch
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=list;get;watch;create;update
// +kubebuilder:rbac:groups="",resources=services,verbs=list;get;watch;create;update
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=list;get;watch;create;update
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=list;get;watch;create;update

// +kubebuilder:rbac:groups=operator.marin3r.3scale.net,resources=discoveryservices,verbs=create

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *NamespaceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	var err error
	result := ctrl.Result{}
	ctx := context.Background()
	l := r.Log.WithValues("namespace", req.NamespacedName.Name)

	// read the object that caused the event
	ns := &v1.Namespace{}
	err = r.Get(ctx, req.NamespacedName, ns)
	if err != nil {
		r.Log.Error(err, "reading Namespace")
		return result, err
	}

	if ns.Status.Phase == v1.NamespaceTerminating {
		l.Info("namespace is Terminating")
		return result, nil
	}

	// Check that the NS has the labels that indicate that it's an EPIC
	// client NameSpace
	if !nsHasLabels(ns, orgLabels) {
		return result, nil
	}

	// Create the Marin3r DiscoveryService that will configure this
	// namespace's Envoys
	if err := maybeCreateMarin3r(ctx, r, l, ns.Name, true); err != nil {
		return result, err
	}

	// fetch the node config; it tells us the EDS image to launch
	config := &egwv1.EGW{}
	err = r.Get(ctx, types.NamespacedName{Name: egwv1.ConfigName, Namespace: egwv1.ConfigNamespace}, config)
	if err != nil {
		return result, err
	}

	// Create the "endpoints" EDS control plane deployment and RBAC
	// cruft
	if err := maybeCreateServiceAccount(ctx, r, l, ns.Name); err != nil {
		return result, err
	}
	if err := maybeCreateRole(ctx, r, l, ns.Name); err != nil {
		return result, err
	}
	if err := maybeCreateRoleBinding(ctx, r, l, ns.Name); err != nil {
		return result, err
	}
	if err := maybeCreateService(ctx, r, l, ns.Name); err != nil {
		return result, err
	}
	if err := maybeCreateDeployment(ctx, r, l, ns.Name, config.Spec.EDSImage); err != nil {
		return result, err
	}

	return result, err
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *NamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Namespace{}).
		Complete(r)
}

// nsHasLabels indicates whether the provided namespace has the
// provided labels.
//
// FIXME: we should be able to do this with a filter on the
// controller's Watch.
func nsHasLabels(o *v1.Namespace, labels map[string]string) bool {
	for k, v := range labels {
		if !nsHasLabel(o, k, v) {
			return false
		}
	}
	return true
}

func nsHasLabel(o *v1.Namespace, label, value string) bool {
	for k, v := range o.Labels {
		if k == label && v == value {
			return true
		}
	}
	return false
}

// maybeCreateMarin3r creates a new marin3r.DiscoveryService if one
// doesn't exist, or does nothing if one already exists.
func maybeCreateMarin3r(ctx context.Context, cl client.Client, l logr.Logger, namespace string, debug bool) error {
	ds := marin3r.DiscoveryService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "discoveryservice",
			Namespace: namespace,
		},
		Spec: marin3r.DiscoveryServiceSpec{
			Debug: pointer.BoolPtr(true),
		},
	}

	if err := maybeCreate(ctx, cl, &ds); err != nil {
		l.Info("Failed to create new DiscoveryService", "message", err.Error(), "name", ds.Name)
		return err
	}

	l.Info("DiscoveryService created", "name", ds.Name)
	return nil
}

// maybeCreateServiceAccount creates a new ServiceAccount if one
// doesn't exist, or does nothing if one already exists.
func maybeCreateServiceAccount(ctx context.Context, cl client.Client, l logr.Logger, namespace string) error {
	sa := v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "endpoints",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "egw",
			},
		},
	}

	if err := maybeCreate(ctx, cl, &sa); err != nil {
		l.Info("Failed to create new ServiceAccount", "message", err.Error(), "name", sa.Name)
		return err
	}

	l.Info("ServiceAccount created", "name", sa.Name)
	return nil
}

// maybeCreateRole creates a new Role (with the permissions that our
// endpoints service needs) if one doesn't exist, or does nothing if
// one already exists.
func maybeCreateRole(ctx context.Context, cl client.Client, l logr.Logger, namespace string) error {
	role := rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "endpoints",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "egw",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"egw.acnodal.io"},
				Resources: []string{"loadbalancers", "remoteendpoints", "servicegroups"},
				Verbs:     []string{"get", "list", "watch", "update", "patch"},
			},
			{
				APIGroups: []string{"egw.acnodal.io"},
				Resources: []string{"servicegroups/status"},
				Verbs:     []string{"get", "update", "patch"},
			},
		},
	}

	if err := maybeCreate(ctx, cl, &role); err != nil {
		l.Info("Failed to create new Role", "message", err.Error(), "name", role.Name)
		return err
	}

	l.Info("Role created", "name", role.Name)
	return nil
}

// maybeCreateRoleBinding creates a new RoleBinding (that binds the
// Role to the ServiceAccount) if one doesn't exist, or does nothing
// if one already exists.
func maybeCreateRoleBinding(ctx context.Context, cl client.Client, l logr.Logger, namespace string) error {
	binding := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "endpoints",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "egw",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "endpoints",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "endpoints",
				Namespace: namespace,
			},
		},
	}

	if err := maybeCreate(ctx, cl, &binding); err != nil {
		l.Info("Failed to create new RoleBinding", "message", err.Error(), "name", binding.Name)
		return err
	}

	l.Info("RoleBinding created", "name", binding.Name)
	return nil
}

// maybeCreateService creates a new Service if one doesn't exist, or
// does nothing if one already exists.
func maybeCreateService(ctx context.Context, cl client.Client, l logr.Logger, namespace string) error {
	svc := v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "epic",
			Namespace: namespace,
			Labels: map[string]string{
				"app":       "egw",
				"component": "endpoints",
			},
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{
				"app":       "egw",
				"component": "endpoints",
			},
			ClusterIP: "None",
		},
	}

	if err := maybeCreate(ctx, cl, &svc); err != nil {
		l.Info("Failed to create new Service", "message", err.Error(), "name", svc.Name)
		return err
	}

	l.Info("Service created", "name", svc.Name)
	return nil
}

// maybeCreateDeployment creates a new Deployment of our EDS server if
// one doesn't exist, or does nothing if one already exists.
func maybeCreateDeployment(ctx context.Context, cl client.Client, l logr.Logger, namespace string, edsImage string) error {
	labels := map[string]string{
		"app":       "egw",
		"component": "endpoints",
	}

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "endpoints",
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: v1.PodSpec{
					Hostname:  "endpoints",
					Subdomain: "epic",
					ImagePullSecrets: []v1.LocalObjectReference{
						{Name: "gitlab"},
					},
					Containers: []v1.Container{{
						Name:            "endpoints",
						Image:           edsImage,
						ImagePullPolicy: v1.PullAlways,
						Env: []v1.EnvVar{{
							Name: "WATCH_NAMESPACE",
							ValueFrom: &v1.EnvVarSource{
								FieldRef: &v1.ObjectFieldSelector{
									FieldPath: "metadata.namespace",
								},
							},
						}},
						Ports: []v1.ContainerPort{
							{ContainerPort: 8080},
							{ContainerPort: 18000},
						},
						SecurityContext: &v1.SecurityContext{
							AllowPrivilegeEscalation: pointer.BoolPtr(true),
							ReadOnlyRootFilesystem:   pointer.BoolPtr(true),
						},
						VolumeMounts: []v1.VolumeMount{
							{
								Name:      "server-cert",
								MountPath: "/etc/envoy/tls/server/",
								ReadOnly:  true,
							},
							{
								Name:      "ca-cert",
								MountPath: "/etc/envoy/tls/ca/",
								ReadOnly:  true,
							},
						},
					}},
					ServiceAccountName:            "endpoints",
					TerminationGracePeriodSeconds: pointer.Int64Ptr(0),
					Volumes: []v1.Volume{
						{
							Name: "server-cert",
							VolumeSource: v1.VolumeSource{
								Secret: &v1.SecretVolumeSource{
									DefaultMode: pointer.Int32Ptr(420),
									SecretName:  "marin3r-server-cert-discoveryservice",
								},
							},
						},
						{
							Name: "ca-cert",
							VolumeSource: v1.VolumeSource{
								Secret: &v1.SecretVolumeSource{
									DefaultMode: pointer.Int32Ptr(420),
									SecretName:  "marin3r-ca-cert-discoveryservice",
								},
							},
						},
					},
				},
			},
		},
	}

	if err := maybeCreate(ctx, cl, &deployment); err != nil {
		l.Info("Failed to create new Deployment", "message", err.Error(), "name", deployment.Name)
		return err
	}

	l.Info("Deployment created", "name", deployment.Name)
	return nil
}

// maybeCreate creates obj if it doesn't exist, or does nothing if it
// already exists.
func maybeCreate(ctx context.Context, cl client.Client, obj runtime.Object) error {
	if err := cl.Create(ctx, obj); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return err
		}
	}

	return nil
}
