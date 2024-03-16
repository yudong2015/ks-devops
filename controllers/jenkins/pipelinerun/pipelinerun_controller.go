/*
Copyright 2020 The KubeSphere Authors.

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

package pipelinerun

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/jenkins-zh/jenkins-client/pkg/core"
	"github.com/jenkins-zh/jenkins-client/pkg/job"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kubesphere.io/devops/pkg/api/devops/v1alpha3"
	devopsClient "kubesphere.io/devops/pkg/client/devops"
	"kubesphere.io/devops/pkg/jwt/token"
	"kubesphere.io/devops/pkg/utils/k8sutil"
)

// tokenExpireIn indicates that the temporary token issued by controller will be expired in some time.
const tokenExpireIn time.Duration = 5 * time.Minute

// Reconciler reconciles a PipelineRun object
type Reconciler struct {
	client.Client
	log          logr.Logger
	Scheme       *runtime.Scheme
	DevOpsClient devopsClient.Interface
	JenkinsCore  core.JenkinsCore
	TokenIssuer  token.Issuer
	recorder     record.EventRecorder
}

//+kubebuilder:rbac:groups=devops.kubesphere.io,resources=pipelineruns,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=devops.kubesphere.io,resources=pipelineruns/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.log.WithValues("PipelineRun", req.NamespacedName)

	// get PipelineRun
	pipelineRun := &v1alpha3.PipelineRun{}
	var err error
	if err = r.Client.Get(ctx, req.NamespacedName, pipelineRun); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.V(5).Info("## reconcile pipelinerun","resource version", pipelineRun.GetResourceVersion())

	jHandler := &jenkinsHandler{&r.JenkinsCore}

	// don't modify the cache in other places, like informer cache.
	pipelineRunCopied := pipelineRun.DeepCopy()

	// DeletionTimestamp.IsZero() means copyPipeline has not been deleted.
	if !pipelineRunCopied.ObjectMeta.DeletionTimestamp.IsZero() {
		if err = jHandler.deleteJenkinsJobHistory(pipelineRunCopied); err != nil {
			klog.V(4).Infof("failed to delete Jenkins job history from PipelineRun: %s/%s, error: %v",
				pipelineRunCopied.Namespace, pipelineRunCopied.Name, err)
		} else {
			k8sutil.RemoveFinalizer(&pipelineRunCopied.ObjectMeta, v1alpha3.PipelineRunFinalizerName)
			err = r.Update(context.TODO(), pipelineRunCopied)
		}
		return ctrl.Result{}, err
	}

	// the PipelineRun cannot allow building
	if !pipelineRunCopied.Buildable() {
		return ctrl.Result{}, nil
	}

	// check PipelineRef
	if pipelineRunCopied.Spec.PipelineRef == nil || pipelineRunCopied.Spec.PipelineRef.Name == "" {
		// make the PipelineRun as orphan
		return ctrl.Result{}, r.makePipelineRunOrphan(ctx, pipelineRunCopied)
	}

	// get pipeline
	pipeline := &v1alpha3.Pipeline{}
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: pipelineRunCopied.Namespace, Name: pipelineRunCopied.Spec.PipelineRef.Name}, pipeline); err != nil {
		log.Error(err, "unable to get pipeline")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	namespaceName := pipeline.Namespace
	pipelineName := pipeline.GetName()

	// set Pipeline name and SCM ref name into labels
	if pipelineRunCopied.Labels == nil {
		pipelineRunCopied.Labels = make(map[string]string)
	}
	pipelineRunCopied.Labels[v1alpha3.PipelineNameLabelKey] = pipelineName

	log = log.WithValues("namespace", namespaceName, "Pipeline", pipelineName)

	// check PipelineRun status
	if pipelineRunCopied.HasStarted() {
		log.V(5).Info("pipeline has already started, and we are retrieving run data from Jenkins.")
		pipelineBuild, err := jHandler.getPipelineRunResult(namespaceName, pipelineName, pipelineRunCopied)
		if err != nil {
			log.Error(err, "unable get PipelineRun data.")
			r.recorder.Eventf(pipelineRunCopied, corev1.EventTypeWarning, v1alpha3.RetrieveFailed, "Failed to retrieve running data from Jenkins, and error was %v", err)
		} else {
			status := pipelineRunCopied.Status.DeepCopy()
			pbApplier := pipelineBuildApplier{pipelineBuild}
			pbApplier.apply(status)

		    log.V(5).Info("## [RUNNING] update status..","resource version", pipelineRunCopied.ResourceVersion)
			// Because the status is a subresource of PipelineRun, we have to update status separately.
			// See also: https://book-v1.book.kubebuilder.io/basics/status_subresource.html
			if err := r.updateStatus(ctx, status, req.NamespacedName); err != nil {
				log.Error(err, "unable to update PipelineRun status.")
				return ctrl.Result{}, err
			}
			log.V(5).Info("## [RUNNING] updated status", "status", fmt.Sprintf("%+v", status))
		}

		log.V(5).Info("## [RUNNING] get node details ..")
		nodeDetails, err := jHandler.getPipelineNodeDetails(pipelineName, namespaceName, pipelineRunCopied)
		if err != nil {
			log.Error(err, "unable to get PipelineRun nodes detail")
			r.recorder.Eventf(pipelineRunCopied, corev1.EventTypeWarning, v1alpha3.RetrieveFailed, "Failed to retrieve nodes detail from Jenkins, and error was %v", err)
		}
		runResultJSON, err := json.Marshal(pipelineBuild)
		if err != nil {
			log.Error(err, "unable to marshal result data to JSON")
			runResultJSON = []byte("{}")
		}
		nodeDetailsJSON, err := json.Marshal(nodeDetails)
		if err != nil {
			log.Error(err, "unable to marshal nodes details to JSON")
			nodeDetailsJSON = []byte("[]")
		}
		if pipelineRunCopied.Annotations == nil {
			pipelineRunCopied.Annotations = make(map[string]string)
		}
		pipelineRunCopied.Annotations[v1alpha3.JenkinsPipelineRunStatusAnnoKey] = string(runResultJSON)
		pipelineRunCopied.Annotations[v1alpha3.JenkinsPipelineRunStagesStatusAnnoKey] = string(nodeDetailsJSON)

		log.V(5).Info("## [RUNNING] update annotations..", "annotations", fmt.Sprintf("%+v", pipelineRunCopied.Annotations), "resource version", pipelineRunCopied.ResourceVersion)
		// update labels and annotations
		if err := r.updateLabelsAndAnnotations(ctx, pipelineRunCopied); err != nil {
			log.Error(err, "unable to update PipelineRun labels and annotations.")
			return ctrl.Result{}, err
		}
		log.V(5).Info("## [RUNNING] updated annotations", "resource version", pipelineRunCopied.ResourceVersion)

		r.recorder.Eventf(pipelineRunCopied, corev1.EventTypeNormal, v1alpha3.Updated, "Updated running data for PipelineRun %s", req.NamespacedName)
		// until the status is okay
		// TODO make the RequeueAfter configurable
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	}

	log.V(5).Info("## pipelinerun not start..")
	// get or create JenkinsCore if the PipelineRun has creator annotation
	jenkinsCore, err := r.getOrCreateJenkinsCore(pipelineRunCopied.GetAnnotations())
	if err != nil {
		r.recorder.Eventf(pipelineRunCopied, corev1.EventTypeWarning, v1alpha3.TriggerFailed, "Failed to trigger PipelineRun %s, and error was %v", req.NamespacedName, err)
		return ctrl.Result{}, err
	}
	// create trigger handler
	triggerHandler := &jenkinsHandler{jenkinsCore}
	// first run
	log.V(5).Info("## pipelinerun first run, trigger..")
	jobRun, err := triggerHandler.triggerJenkinsJob(namespaceName, pipelineName, &pipelineRunCopied.Spec)
	if err != nil {
		log.Error(err, "unable to run pipeline", "namespace", namespaceName, "pipeline", pipeline.Name)
		now := time.Now().Unix()
		msg := fmt.Sprintf("[%d] user: %s, token: %s", now, triggerHandler.UserName, triggerHandler.Token)
		log.Info(msg)
		r.recorder.Eventf(pipelineRunCopied, corev1.EventTypeWarning, v1alpha3.TriggerFailed, "Failed to trigger PipelineRun %s, and error was %v", req.NamespacedName, err)
		return ctrl.Result{}, err
	}
	// check if there is still a same PipelineRun
	log.V(5).Info("## check has same pipelinerun")
	if exists, err := r.hasSamePipelineRun(jobRun, pipeline); err != nil {
		return ctrl.Result{}, err
	} else if exists {
		// if there still exists the same pending PipelineRun, then give up reconciling
		if err := r.Delete(ctx, pipelineRunCopied); err != nil {
			// ignore the not found error here
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		log.Info("Skipped this PipelineRun because there was still a pending Pipeline with the same parameter")
		return ctrl.Result{}, nil
	}

	log.Info("Triggered a PipelineRun", "runID", jobRun.ID)

	// set Jenkins run ID
	if pipelineRunCopied.Annotations == nil {
		pipelineRunCopied.Annotations = make(map[string]string)
	}
	pipelineRunCopied.Annotations[v1alpha3.JenkinsPipelineRunIDAnnoKey] = jobRun.ID

	log.V(5).Info("## [FIRST] update labels and annotations..", "resource version", pipelineRunCopied.ResourceVersion)
	// the Update method only updates fields except subresource: status
	if err := r.updateLabelsAndAnnotations(ctx, pipelineRunCopied); err != nil {
		log.Error(err, "unable to update PipelineRun labels and annotations.")
		return ctrl.Result{}, err
	}
	log.V(5).Info("## [FIRST] after update labels and annotations", "resource version", pipelineRunCopied.ResourceVersion, "annotations", fmt.Sprintf("%+v", pipelineRunCopied.Annotations))

	pipelineRunCopied.Status.StartTime = &v1.Time{Time: time.Now()}
	pipelineRunCopied.Status.UpdateTime = &v1.Time{Time: time.Now()}
	// due to the status is subresource of PipelineRun, we have to update status separately.
	// see also: https://book-v1.book.kubebuilder.io/basics/status_subresource.html

	log.V(5).Info("## [FIRST] update status..","resource version", pipelineRunCopied.ResourceVersion)
	if err := r.updateStatus(ctx, &pipelineRunCopied.Status, req.NamespacedName); err != nil {
		log.Error(err, "unable to update PipelineRun status.")
		return ctrl.Result{}, err
	}
	log.V(5).Info("## [FIRST] after update status", "status", fmt.Sprintf("%+v", pipelineRunCopied.Status))
	r.recorder.Eventf(pipelineRunCopied, corev1.EventTypeNormal, v1alpha3.Started, "Started PipelineRun %s", req.NamespacedName)
	// requeue after 1 second
	return ctrl.Result{}, nil
}

func (r *Reconciler) hasSamePipelineRun(jobRun *job.PipelineRun, pipeline *v1alpha3.Pipeline) (exists bool, err error) {
	// check if the run ID exists in the PipelineRun
	pipelineRuns := &v1alpha3.PipelineRunList{}
	listOptions := []client.ListOption{
		client.InNamespace(pipeline.Namespace),
		client.MatchingLabels{v1alpha3.PipelineNameLabelKey: pipeline.Name},
	}
	if pipeline.Spec.Type == v1alpha3.MultiBranchPipelineType {
		// add SCM reference name into list options for multi-branch Pipeline
		listOptions = append(listOptions, client.MatchingFields{v1alpha3.PipelineRunSCMRefNameField: jobRun.Pipeline})
	}
	if err = r.Client.List(context.Background(), pipelineRuns, listOptions...); err == nil {
		isMultiBranch := pipeline.Spec.Type == v1alpha3.MultiBranchPipelineType
		finder := newPipelineRunFinder(pipelineRuns.Items)
		_, exists = finder.find(jobRun, isMultiBranch)
	}
	return
}

func getSCMRefName(prSpec *v1alpha3.PipelineRunSpec) (string, error) {
	var branch = ""
	if prSpec.IsMultiBranchPipeline() {
		if prSpec.SCM == nil || prSpec.SCM.RefName == "" {
			return "", fmt.Errorf("failed to obtain SCM reference name for multi-branch Pipeline")
		}
		branch = prSpec.SCM.RefName
	}
	return branch, nil
}

func (r *Reconciler) updateLabelsAndAnnotations(ctx context.Context, pr *v1alpha3.PipelineRun) error {
	// get pipeline
	prToUpdate := v1alpha3.PipelineRun{}
	err := r.Get(ctx, client.ObjectKey{Namespace: pr.Namespace, Name: pr.Name}, &prToUpdate)
	if err != nil {
		return err
	}
	r.log.V(5).Info("## [UPDATING ANNOTATION]", "name", prToUpdate.Name, "version", prToUpdate.ResourceVersion)
	if reflect.DeepEqual(pr.Labels, prToUpdate.Labels) && reflect.DeepEqual(pr.Annotations, prToUpdate.Annotations) {
		return nil
	}
	prToUpdate = *prToUpdate.DeepCopy()
	prToUpdate.Labels = pr.Labels
	prToUpdate.Annotations = pr.Annotations
	// make sure all PipelineRuns have the finalizer
	k8sutil.AddFinalizer(&prToUpdate.ObjectMeta, v1alpha3.PipelineRunFinalizerName)
	if err = r.Update(ctx, &prToUpdate); err != nil {
		return err
	}
	if err = r.Get(ctx, client.ObjectKey{Namespace: pr.Namespace, Name: pr.Name}, &prToUpdate); err != nil {
		return err
	}
	r.log.V(5).Info("## [UPDATED ANNOTATION]", "name", prToUpdate.Name, "version", prToUpdate.ResourceVersion)
	return nil
}

func (r *Reconciler) updateStatus(ctx context.Context, desiredStatus *v1alpha3.PipelineRunStatus, prKey client.ObjectKey) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		prToUpdate := v1alpha3.PipelineRun{}
		err := r.Get(ctx, prKey, &prToUpdate)
		if err != nil {
			return err
		}
		r.log.V(5).Info("## [UPDATING STATUS]", "name", prToUpdate.Name, "version", prToUpdate.ResourceVersion)
		if reflect.DeepEqual(*desiredStatus, prToUpdate.Status) {
			return nil
		}
		prToUpdate = *prToUpdate.DeepCopy()
		prToUpdate.Status = *desiredStatus
		if err = r.Status().Update(ctx, &prToUpdate); err != nil {
			return err
		}
		if err = r.Get(ctx, prKey, &prToUpdate); err != nil {
			return err
		}
		r.log.V(5).Info("## [UPDATED STATUS]", "name", prToUpdate.Name, "version", prToUpdate.ResourceVersion)
		return err
	})
}

func (r *Reconciler) makePipelineRunOrphan(ctx context.Context, pr *v1alpha3.PipelineRun) error {
	// make the PipelineRun as orphan
	prToUpdate := pr.DeepCopy()
	prToUpdate.LabelAsAnOrphan()
	if err := r.updateLabelsAndAnnotations(ctx, prToUpdate); err != nil {
		return err
	}
	condition := v1alpha3.Condition{
		Type:               v1alpha3.ConditionSucceeded,
		Status:             v1alpha3.ConditionUnknown,
		Reason:             "SKIPPED",
		Message:            "skipped to reconcile this PipelineRun due to not found Pipeline reference in PipelineRun.",
		LastTransitionTime: v1.Now(),
		LastProbeTime:      v1.Now(),
	}
	prToUpdate.Status.AddCondition(&condition)
	prToUpdate.Status.Phase = v1alpha3.Unknown
	return r.updateStatus(ctx, &pr.Status, client.ObjectKey{Namespace: pr.Namespace, Name: pr.Name})
}

func (r *Reconciler) getOrCreateJenkinsCore(annotations map[string]string) (*core.JenkinsCore, error) {
	creator, ok := annotations[v1alpha3.PipelineRunCreatorAnnoKey]
	if !ok || creator == "" {
		return &r.JenkinsCore, nil
	}
	// create a new JenkinsCore for current creator
	accessToken, err := r.TokenIssuer.IssueTo(&user.DefaultInfo{Name: creator}, token.AccessToken, tokenExpireIn)
	if err != nil {
		return nil, fmt.Errorf("failed to issue access token for creator %s, error was %v", creator, err)
	}
	jenkinsCore := &core.JenkinsCore{
		URL:      r.JenkinsCore.URL,
		UserName: creator,
		Token:    accessToken,
	}
	return jenkinsCore, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// the name should obey Kubernetes naming convention: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/
	r.recorder = mgr.GetEventRecorderFor("pipelinerun-controller")
	r.log = ctrl.Log.WithName("pipelinerun-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha3.PipelineRun{}).
		Complete(r)
}
