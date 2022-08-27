/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"github.com/robfig/cron/v3"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	workloadv1beta1 "github.com/TOMOFUMI-KONDO/auto-workload/api/v1beta1"
)

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (_ *realClock) Now() time.Time {
	return time.Now()
}

// AutoWorkloadReconciler reconciles a AutoWorkload object
type AutoWorkloadReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Clock
}

//+kubebuilder:rbac:groups=workload.tomo-kon.com,resources=autoworkloads,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workload.tomo-kon.com,resources=autoworkloads/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=workload.tomo-kon.com,resources=autoworkloads/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *AutoWorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconcile started")

	awl := &workloadv1beta1.AutoWorkload{}
	if err := r.Get(ctx, client.ObjectKey{Name: req.Name, Namespace: req.Namespace}, awl); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	nextStart, err := r.next(awl.Spec.StartAt)
	if err != nil {
		logger.Error(err, "Failed to calculate next start")
		return ctrl.Result{}, err
	}

	nextStop, err := r.next(awl.Spec.StopAt)
	if err != nil {
		logger.Error(err, "Failed to calculate next stop")
		return ctrl.Result{}, err
	}

	if nextStart.Before(*nextStop) {
		if err := r.createDeployment(awl); err != nil {
			logger.Error(err, "Failed to createDeployment")
			return ctrl.Result{}, err
		}
	} else {
		if err := r.deleteDeployment(awl); err != nil {
			logger.Error(err, "Failed to deleteDeployment")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AutoWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Clock == nil {
		r.Clock = &realClock{}
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&workloadv1beta1.AutoWorkload{}).
		Complete(r)
}

func (r *AutoWorkloadReconciler) createDeployment(awl *workloadv1beta1.AutoWorkload) error {

}

func (r *AutoWorkloadReconciler) deleteDeployment(awl *workloadv1beta1.AutoWorkload) error {

}

func (r *AutoWorkloadReconciler) next(cronExp string) (*time.Time, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(cronExp)
	if err != nil {
		return nil, err
	}

	next := schedule.Next(r.Now())
	return &next, nil
}
