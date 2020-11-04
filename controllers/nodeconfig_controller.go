package controllers

import (
	"context"
	"os/exec"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	egwv1 "gitlab.com/acnodal/egw-resource-model/api/v1"
)

// NodeConfigReconciler reconciles a NodeConfig object
type NodeConfigReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=egw.acnodal.io,resources=nodeconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=egw.acnodal.io,resources=nodeconfigs/status,verbs=get;update;patch

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *NodeConfigReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	var err error
	result := ctrl.Result{}
	ctx := context.Background()
	log := r.Log.WithValues("nodeconfig", req.NamespacedName)

	// read the object that caused the event
	nc := &egwv1.NodeConfig{}
	err = r.Get(ctx, req.NamespacedName, nc)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizer

			// Return and don't requeue
			log.Info("resource not found. Ignoring since object must be deleted")
			return result, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get resource, will retry")
		return result, err
	}

	base := nc.Spec.Base
	for _, nic := range base.IngressNICs {
		err = setupNIC(nic)
		if err != nil {
			log.Error(err, "Failed to setup NIC "+nic)
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *NodeConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&egwv1.NodeConfig{}).
		Complete(r)
}

func setupNIC(nic string) error {
	var err error
	log := ctrl.Log.WithName(nic)

	// tc qdisc add dev nic clsact
	err = addQueueDiscipline(nic)
	if err == nil {
		log.Info("qdisc added")
	} else {
		log.Error(err, "qdisc add error")
	}

	// tc filter add dev nic ingress bpf direct-action object-file pfc_ingress_tc.o sec .text
	err = addIngressFilter(nic)
	if err == nil {
		log.Info("ingress filter added")
	} else {
		log.Error(err, "ingress filter add error")
	}

	// ./cli_cfg set nic 0 0 9 "nic rx"
	err = configurePFC(nic)
	if err == nil {
		log.Info("pfc configured")
	} else {
		log.Error(err, "pfc configuration error")
	}

	return nil
}

func addQueueDiscipline(nic string) error {
	cmd := exec.Command("tc", "qdisc", "add", "dev", nic, "clsact")
	return cmd.Run()
}

func addIngressFilter(nic string) error {
	cmd := exec.Command("tc", "filter", "add", "dev", nic, "ingress", "bpf", "direct-action", "object-file", "/opt/acnodal/bin/pfc_ingress_tc.o", "sec", ".text")
	return cmd.Run()
}

func configurePFC(nic string) error {
	cmd := exec.Command("/opt/acnodal/bin/cli_cfg", "set", nic, "0", "0", "9", "\""+nic+" rx\"")
	return cmd.Run()
}
