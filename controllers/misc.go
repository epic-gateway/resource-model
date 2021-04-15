package controllers

import (
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	done     = ctrl.Result{Requeue: false}
	tryAgain = ctrl.Result{RequeueAfter: 10 * time.Second}
)
