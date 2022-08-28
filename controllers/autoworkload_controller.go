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
	"fmt"
	"github.com/robfig/cron/v3"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

type OperationName string

const (
	OpStart OperationName = "start"
	OpStop  OperationName = "stop"
)

type Operation struct {
	Name OperationName
	*time.Time
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
	logger.Info("Reconcile started!")

	awl := &workloadv1beta1.AutoWorkload{}
	if err := r.Get(ctx, client.ObjectKey{Name: req.Name, Namespace: req.Namespace}, awl); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	prevStatus := awl.Status.DeepCopy()

	if awl.Status.NextStartAt == nil {
		op := &Operation{Name: OpStart}
		if err := r.setNextSchedule(ctx, op, awl); err != nil {
			logger.Error(err, "Failed to setNextSchedule", "operation", op)
			return ctrl.Result{}, err
		}
		if err := r.updateStatus(ctx, awl, prevStatus); err != nil {
			logger.Error(err, "Failed to updateStatus")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if awl.Status.NextStopAt == nil {
		op := &Operation{Name: OpStop}
		if err := r.setNextSchedule(ctx, op, awl); err != nil {
			logger.Error(err, "Failed to setNextSchedule", "operation", op)
			return ctrl.Result{}, err
		}
		if err := r.updateStatus(ctx, awl, prevStatus); err != nil {
			logger.Error(err, "Failed to updateStatus")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	var priorOp Operation
	var postOp Operation
	if awl.Status.NextStartAt.Before(awl.Status.NextStopAt) {
		priorOp = Operation{Name: OpStart, Time: &awl.Status.NextStartAt.Time}
		postOp = Operation{Name: OpStop, Time: &awl.Status.NextStopAt.Time}
	} else {
		priorOp = Operation{Name: OpStop, Time: &awl.Status.NextStopAt.Time}
		postOp = Operation{Name: OpStart, Time: &awl.Status.NextStartAt.Time}
	}

	var requeueAfter time.Duration
	now := r.Now()
	if now.Before(*priorOp.Time) {
		logger.Info("Requeue to reconcile at next operation time", "next operation time", *priorOp.Time)
		return ctrl.Result{Requeue: true, RequeueAfter: priorOp.Time.Sub(now)}, nil
	} else if now.Before(*postOp.Time) {
		if err := r.execOp(ctx, &priorOp, awl); err != nil {
			logger.Error(err, "Failed to execOp", "operation", priorOp)
			return ctrl.Result{}, err
		}

		if err := r.setNextSchedule(ctx, &postOp, awl); err != nil {
			logger.Error(err, "Failed to setNextSchedule", "operation", postOp)
			return ctrl.Result{}, err
		}
		requeueAfter = postOp.Sub(r.Now())
	} else {
		if err := r.execOp(ctx, &postOp, awl); err != nil {
			logger.Error(err, "Failed to execOp", "operation", postOp)
			return ctrl.Result{}, err
		}

		if err := r.setNextSchedule(ctx, &priorOp, awl); err != nil {
			logger.Error(err, "Failed to setNextSchedule", "operation", priorOp)
			return ctrl.Result{}, err
		}
		if err := r.setNextSchedule(ctx, &postOp, awl); err != nil {
			logger.Error(err, "Failed to setNextSchedule", "operation", postOp)
			return ctrl.Result{}, err
		}
		requeueAfter = priorOp.Sub(r.Now())
	}

	if err := r.updateStatus(ctx, awl, prevStatus); err != nil {
		logger.Error(err, "Failed to updateStatus")
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true, RequeueAfter: requeueAfter}, nil
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

func (r *AutoWorkloadReconciler) execOp(ctx context.Context, op *Operation, awl *workloadv1beta1.AutoWorkload) error {
	logger := log.FromContext(ctx)
	logger.Info("Operation started!", "operation name", op.Name)

	switch op.Name {
	case OpStart:
		if err := r.createDeployment(ctx, awl); err != nil {
			logger.Error(err, "Failed to createDeployment")
			return err
		}
	case OpStop:
		if err := r.deleteDeployment(ctx, awl); err != nil {
			logger.Error(err, "Failed to deleteDeployment")
			return err
		}
	}

	logger.Info("Operation completed!", "operation name", op.Name)
	return nil
}

func (r *AutoWorkloadReconciler) createDeployment(ctx context.Context, awl *workloadv1beta1.AutoWorkload) error {
	logger := log.FromContext(ctx)
	logger.Info("createDeployment started!")

	deployment := &appsv1.Deployment{}
	deployment.SetName(awl.Spec.Template.Name)
	deployment.SetNamespace(awl.Spec.Template.Namespace)

	result, err := ctrl.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		deployment = awl.Spec.Template
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to CreateOrUpdate")
		return err
	}
	logger.Info("CreateOrUpdate completed!", "result", result)

	logger.Info("createDeployment completed!")
	return nil
}

func (r *AutoWorkloadReconciler) deleteDeployment(ctx context.Context, awl *workloadv1beta1.AutoWorkload) error {
	logger := log.FromContext(ctx)
	logger.Info("deleteDeployment started!")

	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKeyFromObject(awl.Spec.Template), deployment)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	uid := deployment.GetUID()
	version := deployment.GetResourceVersion()
	cond := &metav1.Preconditions{
		UID:             &uid,
		ResourceVersion: &version,
	}
	option := &client.DeleteOptions{
		Preconditions: cond,
	}
	if err = r.Delete(ctx, deployment, option); err != nil {
		logger.Error(err, "Failed to Delete deployment", "Preconditions", cond)
		return err
	}

	logger.Info("deleteDeployment completed!")
	return nil
}

func (r *AutoWorkloadReconciler) setNextSchedule(ctx context.Context, op *Operation, awl *workloadv1beta1.AutoWorkload) error {
	logger := log.FromContext(ctx)
	logger.Info("setNextSchedule started!")

	var cronExp string
	switch op.Name {
	case OpStart:
		cronExp = awl.Spec.StartAt
	case OpStop:
		cronExp = awl.Spec.StopAt
	default:
		err := fmt.Errorf("operation name invalid: %s", op.Name)
		logger.Error(err, "")
		return err
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(cronExp)
	if err != nil {
		logger.Error(err, "Failed to parse cron expression", "cronExp", cronExp)
		return err
	}

	next := schedule.Next(r.Now())
	metav1Next := metav1.NewTime(next)
	switch op.Name {
	case OpStart:
		awl.Status.NextStartAt = &metav1Next
	case OpStop:
		awl.Status.NextStopAt = &metav1Next
	}

	logger.Info("setNextSchedule completed!")
	return nil
}

func (r *AutoWorkloadReconciler) updateStatus(ctx context.Context, awl *workloadv1beta1.AutoWorkload, prev *workloadv1beta1.AutoWorkloadStatus) error {
	logger := log.FromContext(ctx)
	logger.Info("updateStatus started!")

	if awl.Status == *prev {
		logger.Info("Status has not been changed")
		return nil
	}

	if err := r.Status().Update(ctx, awl); err != nil {
		logger.Error(err, "failed to Update status")
		return err
	}

	return nil
}
