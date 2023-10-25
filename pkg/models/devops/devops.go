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

package devops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	"kubesphere.io/devops/pkg/constants"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"kubesphere.io/devops/pkg/api/devops/v1alpha3"
	devopsv1alpha3 "kubesphere.io/devops/pkg/api/devops/v1alpha3"
	"kubesphere.io/devops/pkg/utils/secretutil"

	"kubesphere.io/devops/pkg/api"
	devopsapi "kubesphere.io/devops/pkg/api/devops"
	"kubesphere.io/devops/pkg/apiserver/query"
	kubesphere "kubesphere.io/devops/pkg/client/clientset/versioned"
	"kubesphere.io/devops/pkg/client/devops"
	resourcesV1alpha3 "kubesphere.io/devops/pkg/models/resources/v1alpha3"
)

const (
	channelMaxCapacity = 100
)

type DevopsOperator interface {
	CreateDevOpsProject(workspace string, project *v1alpha3.DevOpsProject) (*v1alpha3.DevOpsProject, error)
	GetDevOpsProject(workspace string, projectName string) (*v1alpha3.DevOpsProject, error)
	GetDevOpsProjectByGenerateName(workspace string, projectName string) (*v1alpha3.DevOpsProject, error)
	DeleteDevOpsProject(workspace string, projectName string) error
	UpdateDevOpsProject(workspace string, project *v1alpha3.DevOpsProject) (*v1alpha3.DevOpsProject, error)
	ListDevOpsProject(workspace string, limit, offset int) (api.ListResult, error)
	CheckDevopsProject(workspace, projectName string) (map[string]interface{}, error)

	CreatePipelineObj(projectName string, pipeline *v1alpha3.Pipeline) (*v1alpha3.Pipeline, error)
	GetPipelineObj(projectName string, pipelineName string) (*v1alpha3.Pipeline, error)
	DeletePipelineObj(projectName string, pipelineName string) error
	UpdatePipelineObj(projectName string, pipeline *v1alpha3.Pipeline) (*v1alpha3.Pipeline, error)
	ListPipelineObj(projectName string, query *query.Query) (api.ListResult, error)
	UpdateJenkinsfile(projectName, pipelineName, mode, jenkinsfile string) error

	CreateCredentialObj(projectName string, s *v1.Secret) (*v1.Secret, error)
	GetCredentialObj(projectName string, secretName string) (*v1.Secret, error)
	DeleteCredentialObj(projectName string, secretName string) error
	UpdateCredentialObj(projectName string, secret *v1.Secret) (*v1.Secret, error)
	ListCredentialObj(projectName string, query *query.Query) (api.ListResult, error)

	CheckPipelineName(projectName, pipelineName string, req *http.Request) (map[string]interface{}, error)
	GetPipeline(projectName, pipelineName string, req *http.Request) (*devops.Pipeline, error)
	ListPipelines(req *http.Request) (*devops.PipelineList, error)
	GetPipelineRun(projectName, pipelineName, runId string, req *http.Request) (*devops.PipelineRun, error)
	ListPipelineRuns(projectName, pipelineName string, req *http.Request) (*devops.PipelineRunList, error)
	StopPipeline(projectName, pipelineName, runId string, req *http.Request) (*devops.StopPipeline, error)
	ReplayPipeline(projectName, pipelineName, runId string, req *http.Request) (*devops.ReplayPipeline, error)
	RunPipeline(projectName, pipelineName string, req *http.Request) (*devops.RunPipeline, error)
	GetArtifacts(projectName, pipelineName, runId string, req *http.Request) ([]devops.Artifacts, error)
	GetRunLog(projectName, pipelineName, runId string, req *http.Request) ([]byte, http.Header, error)
	GetStepLog(projectName, pipelineName, runId, nodeId, stepId string, req *http.Request) ([]byte, http.Header, error)
	GetNodeSteps(projectName, pipelineName, runId, nodeId string, req *http.Request) ([]devops.NodeSteps, error)
	GetPipelineRunNodes(projectName, pipelineName, runId string, req *http.Request) ([]devops.PipelineRunNodes, error)
	SubmitInputStep(projectName, pipelineName, runId, nodeId, stepId string, req *http.Request) ([]byte, error)
	GetNodesDetail(projectName, pipelineName, runId string, req *http.Request) ([]devops.NodesDetail, error)

	GetBranchPipeline(projectName, pipelineName, branchName string, req *http.Request) (*devops.BranchPipeline, error)
	GetBranchPipelineRun(projectName, pipelineName, branchName, runId string, req *http.Request) (*devops.PipelineRun, error)
	StopBranchPipeline(projectName, pipelineName, branchName, runId string, req *http.Request) (*devops.StopPipeline, error)
	ReplayBranchPipeline(projectName, pipelineName, branchName, runId string, req *http.Request) (*devops.ReplayPipeline, error)
	RunBranchPipeline(projectName, pipelineName, branchName string, req *http.Request) (*devops.RunPipeline, error)
	GetBranchArtifacts(projectName, pipelineName, branchName, runId string, req *http.Request) ([]devops.Artifacts, error)
	GetBranchRunLog(projectName, pipelineName, branchName, runId string, req *http.Request) ([]byte, error)
	GetBranchStepLog(projectName, pipelineName, branchName, runId, nodeId, stepId string, req *http.Request) ([]byte, http.Header, error)
	GetBranchNodeSteps(projectName, pipelineName, branchName, runId, nodeId string, req *http.Request) ([]devops.NodeSteps, error)
	GetBranchPipelineRunNodes(projectName, pipelineName, branchName, runId string, req *http.Request) ([]devops.BranchPipelineRunNodes, error)
	SubmitBranchInputStep(projectName, pipelineName, branchName, runId, nodeId, stepId string, req *http.Request) ([]byte, error)
	GetBranchNodesDetail(projectName, pipelineName, branchName, runId string, req *http.Request) ([]devops.NodesDetail, error)
	GetPipelineBranch(projectName, pipelineName string, req *http.Request) (*devops.PipelineBranch, error)
	ScanBranch(projectName, pipelineName string, req *http.Request) ([]byte, error)

	GetConsoleLog(projectName, pipelineName string, req *http.Request) ([]byte, error)
	GetCrumb(req *http.Request) (*devops.Crumb, error)

	GetSCMServers(scmId string, req *http.Request) ([]devops.SCMServer, error)
	GetSCMOrg(scmId string, req *http.Request) ([]devops.SCMOrg, error)
	GetOrgRepo(scmId, organizationId string, req *http.Request) (devops.OrgRepo, error)
	CreateSCMServers(scmId string, req *http.Request) (*devops.SCMServer, error)
	Validate(scmId string, req *http.Request) (*devops.Validates, error)

	GetNotifyCommit(req *http.Request) ([]byte, error)
	GithubWebhook(req *http.Request) ([]byte, error)
	GenericWebhook(req *http.Request) ([]byte, error)

	CheckScriptCompile(projectName, pipelineName string, req *http.Request) (*devops.CheckScript, error)
	CheckCron(projectName string, req *http.Request) (*devops.CheckCronRes, error)

	GetJenkinsAgentLabels() ([]string, error)
}

type devopsOperator struct {
	devopsClient devops.Interface
	k8sclient    kubernetes.Interface
	ksclient     kubesphere.Interface
	context      context.Context
}

func NewDevopsOperator(client devops.Interface,
	k8sclient kubernetes.Interface,
	ksclient kubesphere.Interface) DevopsOperator {
	return &devopsOperator{
		devopsClient: client,
		k8sclient:    k8sclient,
		ksclient:     ksclient,
		context:      context.Background(),
	}
}

func convertToHttpParameters(req *http.Request) *devops.HttpParameters {
	httpParameters := devops.HttpParameters{
		Method:   req.Method,
		Header:   req.Header,
		Body:     req.Body,
		Form:     req.Form,
		PostForm: req.PostForm,
		Url:      req.URL,
	}

	return &httpParameters
}

func (d devopsOperator) CreateDevOpsProject(workspace string, project *v1alpha3.DevOpsProject) (*v1alpha3.DevOpsProject, error) {
	// All resources of devops project belongs to the namespace of the same name
	// The devops project name is used as the name of the admin namespace, using generateName to avoid conflicts
	if project.GenerateName == "" {
		err := errors.NewInvalid(devopsv1alpha3.GroupVersion.WithKind(devopsv1alpha3.ResourceKindDevOpsProject).GroupKind(),
			"", []*field.Error{field.Required(field.NewPath("metadata.generateName"), "generateName is required")})
		klog.Error(err)
		return nil, err
	}
	// generateName is used as displayName
	// ensure generateName is unique in workspace scope
	if unique, err := d.isGenerateNameUnique(workspace, project.GenerateName); err != nil {
		return nil, err
	} else if !unique {
		err = errors.NewConflict(devopsv1alpha3.Resource(devopsv1alpha3.ResourceSingularDevOpsProject),
			project.GenerateName, fmt.Errorf(project.GenerateName, fmt.Errorf("a devops project named %s already exists in the workspace", project.GenerateName)))
		klog.Error(err)
		return nil, err
	}

	// metadata override
	if project.Labels == nil {
		project.Labels = make(map[string]string)
	}
	project.Name = ""
	project.Labels[constants.WorkspaceLabelKey] = workspace

	// set annotations
	if project.Annotations == nil {
		project.Annotations = make(map[string]string)
	}
	project.Annotations[devopsv1alpha3.DevOpeProjectSyncStatusAnnoKey] = StatusPending
	project.Annotations[devopsv1alpha3.DevOpeProjectSyncTimeAnnoKey] = GetSyncNowTime()

	// create it
	return d.ksclient.DevopsV1alpha3().DevOpsProjects().Create(d.context, project, metav1.CreateOptions{})
}

// CheckDevopsProject check the devops is not exist
func (d devopsOperator) CheckDevopsProject(workspace, projectName string) (map[string]interface{}, error) {
	var list *v1alpha3.DevOpsProjectList
	var err error

	result := make(map[string]interface{})
	result["exist"] = false

	if list, err = d.ksclient.DevopsV1alpha3().DevOpsProjects().List(d.context, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", constants.WorkspaceLabelKey, workspace),
	}); err == nil {
		for i := range list.Items {
			item := list.Items[i]
			if item.GenerateName == projectName {
				result["exist"] = true
				break
			}
		}
		return result, nil
	} else {
		return result, err
	}
}

// GetDevOpsProjectByGenerateName finds the DevOps project by workspace and project name
// the projectName is the generateName instead of the real resource name
func (d devopsOperator) GetDevOpsProjectByGenerateName(workspace string, projectName string) (project *v1alpha3.DevOpsProject, err error) {
	var list *v1alpha3.DevOpsProjectList

	if list, err = d.ksclient.DevopsV1alpha3().DevOpsProjects().List(d.context, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", constants.WorkspaceLabelKey, workspace),
	}); err == nil {
		for i := range list.Items {
			item := list.Items[i]
			if item.GenerateName == projectName {
				project = &item
				break
			}
		}

		if project == nil {
			err = fmt.Errorf("not found DevOpsProject by workspace: %s and projectName: %s", workspace, projectName)
		}
	}
	return
}

// GetDevOpsProject finds the DevOps project by workspace and project name
func (d devopsOperator) GetDevOpsProject(workspace string, projectName string) (*v1alpha3.DevOpsProject, error) {
	return d.ksclient.DevopsV1alpha3().DevOpsProjects().Get(d.context, projectName, metav1.GetOptions{})
}

func (d devopsOperator) DeleteDevOpsProject(workspace string, projectName string) error {
	return d.ksclient.DevopsV1alpha3().DevOpsProjects().Delete(d.context, projectName, *metav1.NewDeleteOptions(0))
}

func (d devopsOperator) UpdateDevOpsProject(workspace string, project *v1alpha3.DevOpsProject) (*v1alpha3.DevOpsProject, error) {
	if project.Annotations == nil {
		project.Annotations = make(map[string]string)
	}
	project.Annotations[devopsv1alpha3.DevOpeProjectSyncStatusAnnoKey] = StatusPending
	project.Annotations[devopsv1alpha3.DevOpeProjectSyncTimeAnnoKey] = GetSyncNowTime()
	return d.ksclient.DevopsV1alpha3().DevOpsProjects().Update(d.context, project, metav1.UpdateOptions{})
}

func (d devopsOperator) ListDevOpsProject(workspace string, limit, offset int) (api.ListResult, error) {
	devOpsProjectList, err := d.ksclient.DevopsV1alpha3().DevOpsProjects().List(d.context, metav1.ListOptions{})
	if err != nil {
		return api.ListResult{}, nil
	}
	items := make([]interface{}, 0)
	var result []interface{}
	for _, item := range devOpsProjectList.Items {
		result = append(result, item)
	}

	if limit == -1 || limit+offset > len(result) {
		limit = len(result) - offset
	}
	items = result[offset : offset+limit]
	if items == nil {
		items = []interface{}{}
	}
	return api.ListResult{TotalItems: len(result), Items: items}, nil
}

// pipelineobj in crd
func (d devopsOperator) CreatePipelineObj(projectName string, pipeline *v1alpha3.Pipeline) (pip *v1alpha3.Pipeline, err error) {
	projectObj, err := d.ksclient.DevopsV1alpha3().DevOpsProjects().Get(d.context, projectName, metav1.GetOptions{})
	if err == nil {
		if projectObj.Annotations == nil {
			projectObj.Annotations = map[string]string{}
		}
		projectObj.Annotations[devopsv1alpha3.PipelineSyncStatusAnnoKey] = StatusPending
		projectObj.Annotations[devopsv1alpha3.PipelineSyncTimeAnnoKey] = GetSyncNowTime()
		pip, err = d.ksclient.DevopsV1alpha3().Pipelines(projectObj.Status.AdminNamespace).Create(d.context, pipeline, metav1.CreateOptions{})
	}
	return
}

func (d devopsOperator) GetPipelineObj(projectName string, pipelineName string) (*v1alpha3.Pipeline, error) {
	projectObj, err := d.ksclient.DevopsV1alpha3().DevOpsProjects().Get(d.context, projectName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return d.ksclient.DevopsV1alpha3().Pipelines(projectObj.Status.AdminNamespace).Get(d.context, pipelineName, metav1.GetOptions{})
}

func (d devopsOperator) DeletePipelineObj(projectName string, pipelineName string) error {
	projectObj, err := d.ksclient.DevopsV1alpha3().DevOpsProjects().Get(d.context, projectName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	return d.ksclient.DevopsV1alpha3().Pipelines(projectObj.Status.AdminNamespace).Delete(d.context, pipelineName, *metav1.NewDeleteOptions(0))
}

func (d devopsOperator) UpdatePipelineObj(projectName string, pipeline *v1alpha3.Pipeline) (*v1alpha3.Pipeline, error) {
	projectObj, err := d.ksclient.DevopsV1alpha3().DevOpsProjects().Get(d.context, projectName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	ns := projectObj.Status.AdminNamespace
	name := pipeline.GetName()
	// trying to avoid the error of `Operation cannot be fulfilled on` by getting the latest resourceVersion
	if latestPipe, err := d.ksclient.DevopsV1alpha3().Pipelines(ns).Get(d.context, name, metav1.GetOptions{}); err == nil {
		pipeline.ResourceVersion = latestPipe.ResourceVersion

		// avoid update the Jenkinsfile in this API, see also UpdateJenkinsfile
		if pipeline.Spec.Pipeline != nil && latestPipe.Spec.Pipeline != nil {
			pipeline.Spec.Pipeline.Jenkinsfile = latestPipe.Spec.Pipeline.Jenkinsfile
		}
	} else {
		return nil, fmt.Errorf("cannot found pipeline %s/%s, error: %v", ns, name, err)
	}

	return d.ksclient.DevopsV1alpha3().Pipelines(ns).Update(d.context, pipeline, metav1.UpdateOptions{})
}

// UpdateJenkinsfile updates the Jenkinsfile value with specific edit mode
func (d devopsOperator) UpdateJenkinsfile(projectName, pipelineName, mode, jenkinsfile string) (err error) {
	var pipeline *devopsv1alpha3.Pipeline
	if pipeline, err = d.ksclient.DevopsV1alpha3().Pipelines(projectName).Get(d.context, pipelineName, metav1.GetOptions{}); err != nil {
		return
	}

	if pipeline.Annotations == nil {
		pipeline.Annotations = map[string]string{}
	}
	pipeline.Annotations[devopsv1alpha3.PipelineJenkinsfileEditModeAnnoKey] = mode

	switch mode {
	case devopsv1alpha3.PipelineJenkinsfileEditModeJSON:
		pipeline.Annotations[devopsv1alpha3.PipelineJenkinsfileValueAnnoKey] = jenkinsfile
	case devopsv1alpha3.PipelineJenkinsfileEditModeRaw:
		if pipeline.Spec.Pipeline != nil {
			pipeline.Spec.Pipeline.Jenkinsfile = jenkinsfile
		}
	default:
		err = fmt.Errorf("invalid edit mode: %s", mode)
		return
	}
	_, err = d.ksclient.DevopsV1alpha3().Pipelines(projectName).Update(d.context, pipeline, metav1.UpdateOptions{})
	return
}

func (d devopsOperator) ListPipelineObj(projectName string, queryParam *query.Query) (api.ListResult, error) {
	project, err := d.ksclient.DevopsV1alpha3().DevOpsProjects().Get(d.context, projectName, metav1.GetOptions{})
	if err != nil {
		return api.ListResult{}, err
	}

	pipelines, err := d.ksclient.DevopsV1alpha3().Pipelines(project.Status.AdminNamespace).List(d.context, metav1.ListOptions{
		LabelSelector: queryParam.LabelSelector,
	})

	if err != nil {
		return api.ListResult{}, err
	}

	// filter pipeline type & convert Pipeline to runtime.Object
	pipelineType, typeExist := queryParam.Filters[query.FieldType]
	var result = make([]runtime.Object, len(pipelines.Items))
	for i, _ := range pipelines.Items {
		if typeExist && string(pipelines.Items[i].Spec.Type) != string(pipelineType) {
			continue
		}
		result = append(result, &pipelines.Items[i])
	}

	return *resourcesV1alpha3.DefaultList(result, queryParam, resourcesV1alpha3.DefaultCompare(), resourcesV1alpha3.DefaultFilter()), nil
}

// CreateCredentialObj creates a secret
func (d devopsOperator) CreateCredentialObj(projectName string, secret *v1.Secret) (*v1.Secret, error) {
	projectObj, err := d.ksclient.DevopsV1alpha3().DevOpsProjects().Get(d.context, projectName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	secret.Annotations[devopsv1alpha3.CredentialAutoSyncAnnoKey] = "true"
	secret.Annotations[devopsv1alpha3.CredentialSyncStatusAnnoKey] = StatusPending
	secret.Annotations[devopsv1alpha3.CredentialSyncTimeAnnoKey] = GetSyncNowTime()
	if secret, err := d.k8sclient.CoreV1().Secrets(projectObj.Status.AdminNamespace).Create(d.context, secret, metav1.CreateOptions{}); err != nil {
		return nil, err
	} else {
		return secretutil.MaskCredential(secret), nil
	}
}

func (d devopsOperator) GetCredentialObj(projectName string, secretName string) (*v1.Secret, error) {
	projectObj, err := d.ksclient.DevopsV1alpha3().DevOpsProjects().Get(d.context, projectName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if secret, err := d.k8sclient.CoreV1().Secrets(projectObj.Status.AdminNamespace).Get(d.context, secretName, metav1.GetOptions{}); err != nil {
		return nil, err
	} else {
		return secretutil.MaskCredential(secret), nil
	}
}

func (d devopsOperator) DeleteCredentialObj(projectName string, secret string) error {
	projectObj, err := d.ksclient.DevopsV1alpha3().DevOpsProjects().Get(d.context, projectName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	return d.k8sclient.CoreV1().Secrets(projectObj.Status.AdminNamespace).Delete(d.context, secret, *metav1.NewDeleteOptions(0))
}

func (d devopsOperator) UpdateCredentialObj(projectName string, secret *v1.Secret) (*v1.Secret, error) {
	projectObj, err := d.ksclient.DevopsV1alpha3().DevOpsProjects().Get(d.context, projectName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	secret.Annotations[devopsv1alpha3.CredentialAutoSyncAnnoKey] = "true"
	secret.Annotations[devopsv1alpha3.CredentialSyncStatusAnnoKey] = StatusPending
	secret.Annotations[devopsv1alpha3.CredentialSyncTimeAnnoKey] = GetSyncNowTime()
	if secret, err := d.k8sclient.CoreV1().Secrets(projectObj.Status.AdminNamespace).Update(d.context, secret, metav1.UpdateOptions{}); err != nil {
		return nil, err
	} else {
		return secretutil.MaskCredential(secret), nil
	}
}

func (d devopsOperator) ListCredentialObj(projectName string, query *query.Query) (api.ListResult, error) {
	projectObj, err := d.ksclient.DevopsV1alpha3().DevOpsProjects().Get(d.context, projectName, metav1.GetOptions{})
	if err != nil {
		return api.ListResult{}, err
	}
	credentialObjList, err := d.k8sclient.CoreV1().Secrets(projectObj.Status.AdminNamespace).List(d.context, metav1.ListOptions{
		LabelSelector: query.Selector().String(),
	})
	if err != nil {
		return api.ListResult{}, err
	}
	var result []runtime.Object

	credentialTypeList := v1alpha3.GetSupportedCredentialTypes()
	for i := range credentialObjList.Items {
		credential := credentialObjList.Items[i]
		for _, credentialType := range credentialTypeList {
			if credential.Type == credentialType {
				result = append(result, secretutil.MaskCredential(&credential))
			}
		}
	}

	return *resourcesV1alpha3.DefaultList(result, query, resourcesV1alpha3.DefaultCompare(), resourcesV1alpha3.DefaultFilter()), nil
}

func (d devopsOperator) CheckPipelineName(projectName, pipelineName string, req *http.Request) (map[string]interface{}, error) {
	return d.devopsClient.CheckPipelineName(projectName, pipelineName, convertToHttpParameters(req))
}

// others
func (d devopsOperator) GetPipeline(projectName, pipelineName string, req *http.Request) (*devops.Pipeline, error) {
	return d.devopsClient.GetPipeline(projectName, pipelineName, convertToHttpParameters(req))
}

func (d devopsOperator) ListPipelines(req *http.Request) (*devops.PipelineList, error) {

	res, err := d.devopsClient.ListPipelines(convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
	}
	return res, err
}

func (d devopsOperator) GetPipelineRun(projectName, pipelineName, runId string, req *http.Request) (*devops.PipelineRun, error) {

	res, err := d.devopsClient.GetPipelineRun(projectName, pipelineName, runId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
	}
	return res, err
}

func (d devopsOperator) ListPipelineRuns(projectName, pipelineName string, req *http.Request) (*devops.PipelineRunList, error) {

	res, err := d.devopsClient.ListPipelineRuns(projectName, pipelineName, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
	}
	return res, err
}

func (d devopsOperator) StopPipeline(projectName, pipelineName, runId string, req *http.Request) (*devops.StopPipeline, error) {

	req.Method = http.MethodPut
	res, err := d.devopsClient.StopPipeline(projectName, pipelineName, runId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) ReplayPipeline(projectName, pipelineName, runId string, req *http.Request) (*devops.ReplayPipeline, error) {

	res, err := d.devopsClient.ReplayPipeline(projectName, pipelineName, runId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) RunPipeline(projectName, pipelineName string, req *http.Request) (*devops.RunPipeline, error) {

	res, err := d.devopsClient.RunPipeline(projectName, pipelineName, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) GetArtifacts(projectName, pipelineName, runId string, req *http.Request) ([]devops.Artifacts, error) {

	res, err := d.devopsClient.GetArtifacts(projectName, pipelineName, runId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) GetRunLog(projectName, pipelineName, runId string, req *http.Request) ([]byte, http.Header, error) {

	res, header, err := d.devopsClient.GetRunLog(projectName, pipelineName, runId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, nil, err
	}

	return res, header, err
}

func (d devopsOperator) GetStepLog(projectName, pipelineName, runId, nodeId, stepId string, req *http.Request) ([]byte, http.Header, error) {

	resBody, header, err := d.devopsClient.GetStepLog(projectName, pipelineName, runId, nodeId, stepId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, nil, err
	}

	return resBody, header, err
}

func (d devopsOperator) GetNodeSteps(projectName, pipelineName, runId, nodeId string, req *http.Request) ([]devops.NodeSteps, error) {
	res, err := d.devopsClient.GetNodeSteps(projectName, pipelineName, runId, nodeId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) GetPipelineRunNodes(projectName, pipelineName, runId string, req *http.Request) ([]devops.PipelineRunNodes, error) {
	res, err := d.devopsClient.GetPipelineRunNodes(projectName, pipelineName, runId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) SubmitInputStep(projectName, pipelineName, runId, nodeId, stepId string, req *http.Request) ([]byte, error) {
	newBody, err := getInputReqBody(req.Body)
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	req.Body = newBody

	resBody, err := d.devopsClient.SubmitInputStep(projectName, pipelineName, runId, nodeId, stepId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return resBody, err
}

func (d devopsOperator) GetNodesDetail(projectName, pipelineName, runId string, req *http.Request) ([]devops.NodesDetail, error) {
	var wg sync.WaitGroup
	var nodesDetails []devops.NodesDetail
	stepChan := make(chan *devops.NodesStepsIndex, channelMaxCapacity)

	respNodes, err := d.GetPipelineRunNodes(projectName, pipelineName, runId, req)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	Nodes, err := json.Marshal(respNodes)
	err = json.Unmarshal(Nodes, &nodesDetails)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	// get all steps in nodes.
	for i, v := range respNodes {
		wg.Add(1)
		go func(nodeId string, index int) {
			// We have to clone the request to prevent concurrent header writes in the next process
			Steps, err := d.GetNodeSteps(projectName, pipelineName, runId, nodeId, req.Clone(context.TODO()))
			if err != nil {
				klog.Error(err)
				return
			}

			stepChan <- &devops.NodesStepsIndex{Id: index, Steps: Steps}
			wg.Done()
		}(v.ID, i)
	}

	wg.Wait()
	close(stepChan)

	for oneNodeSteps := range stepChan {
		if oneNodeSteps != nil {
			nodesDetails[oneNodeSteps.Id].Steps = append(nodesDetails[oneNodeSteps.Id].Steps, oneNodeSteps.Steps...)
		}
	}

	return nodesDetails, err
}

func (d devopsOperator) GetBranchPipeline(projectName, pipelineName, branchName string, req *http.Request) (*devops.BranchPipeline, error) {

	res, err := d.devopsClient.GetBranchPipeline(projectName, pipelineName, branchName, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) GetBranchPipelineRun(projectName, pipelineName, branchName, runId string, req *http.Request) (*devops.PipelineRun, error) {

	res, err := d.devopsClient.GetBranchPipelineRun(projectName, pipelineName, branchName, runId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) StopBranchPipeline(projectName, pipelineName, branchName, runId string, req *http.Request) (*devops.StopPipeline, error) {

	req.Method = http.MethodPut
	res, err := d.devopsClient.StopBranchPipeline(projectName, pipelineName, branchName, runId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) ReplayBranchPipeline(projectName, pipelineName, branchName, runId string, req *http.Request) (*devops.ReplayPipeline, error) {

	res, err := d.devopsClient.ReplayBranchPipeline(projectName, pipelineName, branchName, runId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) RunBranchPipeline(projectName, pipelineName, branchName string, req *http.Request) (*devops.RunPipeline, error) {

	res, err := d.devopsClient.RunBranchPipeline(projectName, pipelineName, branchName, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) GetBranchArtifacts(projectName, pipelineName, branchName, runId string, req *http.Request) ([]devops.Artifacts, error) {

	res, err := d.devopsClient.GetBranchArtifacts(projectName, pipelineName, branchName, runId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) GetBranchRunLog(projectName, pipelineName, branchName, runId string, req *http.Request) ([]byte, error) {

	res, err := d.devopsClient.GetBranchRunLog(projectName, pipelineName, branchName, runId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) GetBranchStepLog(projectName, pipelineName, branchName, runId, nodeId, stepId string, req *http.Request) ([]byte, http.Header, error) {

	resBody, header, err := d.devopsClient.GetBranchStepLog(projectName, pipelineName, branchName, runId, nodeId, stepId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, nil, err
	}

	return resBody, header, err
}

func (d devopsOperator) GetBranchNodeSteps(projectName, pipelineName, branchName, runId, nodeId string, req *http.Request) ([]devops.NodeSteps, error) {

	res, err := d.devopsClient.GetBranchNodeSteps(projectName, pipelineName, branchName, runId, nodeId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) GetBranchPipelineRunNodes(projectName, pipelineName, branchName, runId string, req *http.Request) ([]devops.BranchPipelineRunNodes, error) {

	res, err := d.devopsClient.GetBranchPipelineRunNodes(projectName, pipelineName, branchName, runId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) SubmitBranchInputStep(projectName, pipelineName, branchName, runId, nodeId, stepId string, req *http.Request) ([]byte, error) {

	newBody, err := getInputReqBody(req.Body)
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	req.Body = newBody
	resBody, err := d.devopsClient.SubmitBranchInputStep(projectName, pipelineName, branchName, runId, nodeId, stepId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return resBody, err
}

func (d devopsOperator) GetBranchNodesDetail(projectName, pipelineName, branchName, runId string, req *http.Request) ([]devops.NodesDetail, error) {
	var wg sync.WaitGroup
	var nodesDetails []devops.NodesDetail
	stepChan := make(chan *devops.NodesStepsIndex, channelMaxCapacity)

	respNodes, err := d.GetBranchPipelineRunNodes(projectName, pipelineName, branchName, runId, req)
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	Nodes, err := json.Marshal(respNodes)
	err = json.Unmarshal(Nodes, &nodesDetails)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	// get all steps in nodes.
	for i, v := range nodesDetails {
		wg.Add(1)
		go func(nodeId string, index int) {
			Steps, err := d.GetBranchNodeSteps(projectName, pipelineName, branchName, runId, nodeId, req)
			if err != nil {
				klog.Error(err)
				return
			}

			stepChan <- &devops.NodesStepsIndex{Id: index, Steps: Steps}
			wg.Done()
		}(v.ID, i)
	}

	wg.Wait()
	close(stepChan)

	for oneNodeSteps := range stepChan {
		if oneNodeSteps != nil {
			nodesDetails[oneNodeSteps.Id].Steps = append(nodesDetails[oneNodeSteps.Id].Steps, oneNodeSteps.Steps...)
		}
	}

	return nodesDetails, err
}

func (d devopsOperator) GetPipelineBranch(projectName, pipelineName string, req *http.Request) (*devops.PipelineBranch, error) {

	res, err := d.devopsClient.GetPipelineBranch(projectName, pipelineName, convertToHttpParameters(req))
	//baseUrl+req.URL.RawQuery, req)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) ScanBranch(projectName, pipelineName string, req *http.Request) ([]byte, error) {

	resBody, err := d.devopsClient.ScanBranch(projectName, pipelineName, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return resBody, err
}

func (d devopsOperator) GetConsoleLog(projectName, pipelineName string, req *http.Request) ([]byte, error) {

	resBody, err := d.devopsClient.GetConsoleLog(projectName, pipelineName, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return resBody, err
}

func (d devopsOperator) GetCrumb(req *http.Request) (*devops.Crumb, error) {

	res, err := d.devopsClient.GetCrumb(convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) GetSCMServers(scmId string, req *http.Request) ([]devops.SCMServer, error) {

	req.Method = http.MethodGet
	resBody, err := d.devopsClient.GetSCMServers(scmId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
	}
	return resBody, err
}

func (d devopsOperator) GetSCMOrg(scmId string, req *http.Request) ([]devops.SCMOrg, error) {

	res, err := d.devopsClient.GetSCMOrg(scmId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) GetOrgRepo(scmId, organizationId string, req *http.Request) (devops.OrgRepo, error) {

	res, err := d.devopsClient.GetOrgRepo(scmId, organizationId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return devops.OrgRepo{}, err
	}

	return res, err
}

// CreateSCMServers creates a Bitbucket server config item in Jenkins configuration if there's no same API address exist
func (d devopsOperator) CreateSCMServers(scmId string, req *http.Request) (*devops.SCMServer, error) {

	requestBody, err := ioutil.ReadAll(req.Body)
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	createReq := &devops.CreateScmServerReq{}
	err = json.Unmarshal(requestBody, createReq)
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	req.Body = nil
	servers, err := d.GetSCMServers(scmId, req)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	createReq.ApiURL = strings.TrimSuffix(createReq.ApiURL, "/")
	for _, server := range servers {
		if strings.TrimSuffix(server.ApiURL, "/") == createReq.ApiURL {
			return &server, nil
		}
	}
	req.Body = ioutil.NopCloser(bytes.NewReader(requestBody))

	req.Method = http.MethodPost
	resBody, err := d.devopsClient.CreateSCMServers(scmId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	return resBody, err
}

func (d devopsOperator) Validate(scmId string, req *http.Request) (*devops.Validates, error) {

	req.Method = http.MethodPut
	resBody, err := d.devopsClient.Validate(scmId, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return resBody, err
}

func (d devopsOperator) GetNotifyCommit(req *http.Request) ([]byte, error) {

	req.Method = http.MethodGet

	res, err := d.devopsClient.GetNotifyCommit(convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) GithubWebhook(req *http.Request) ([]byte, error) {

	res, err := d.devopsClient.GithubWebhook(convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) GenericWebhook(req *http.Request) (data []byte, err error) {
	res, err := d.devopsClient.GenericWebhook(convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) CheckScriptCompile(projectName, pipelineName string, req *http.Request) (*devops.CheckScript, error) {

	resBody, err := d.devopsClient.CheckScriptCompile(projectName, pipelineName, convertToHttpParameters(req))
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return resBody, err
}

func (d devopsOperator) CheckCron(projectName string, req *http.Request) (*devops.CheckCronRes, error) {

	res, err := d.devopsClient.CheckCron(projectName, convertToHttpParameters(req))

	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return res, err
}

func (d devopsOperator) GetJenkinsAgentLabels() (labels []string, err error) {
	var cm *v1.ConfigMap
	if cm, err = d.k8sclient.CoreV1().ConfigMaps("kubesphere-devops-system").
		Get(context.Background(), "jenkins-agent-config", metav1.GetOptions{}); err == nil {
		labelsInStr := cm.Data[devopsapi.JenkinsAgentLabelsKey]
		labels = strings.Split(labelsInStr, ",")
	}
	return
}

func (d devopsOperator) isGenerateNameUnique(workspace, generateName string) (bool, error) {
	projects, err := d.ksclient.DevopsV1alpha3().DevOpsProjects().List(d.context, metav1.ListOptions{})
	if err != nil {
		klog.Error(err)
		return false, err
	}
	for _, p := range projects.Items {
		if p.Labels != nil && p.Labels[constants.WorkspaceLabelKey] == workspace && p.GenerateName == generateName {
			return false, err
		}
	}
	return true, nil
}

func getInputReqBody(reqBody io.ReadCloser) (newReqBody io.ReadCloser, err error) {
	var checkBody devops.CheckPlayload
	var jsonBody []byte
	var workRound struct {
		ID         string                           `json:"id,omitempty" description:"id"`
		Parameters []devops.CheckPlayloadParameters `json:"parameters"`
		Abort      bool                             `json:"abort,omitempty" description:"abort or not"`
	}

	Body, err := ioutil.ReadAll(reqBody)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	err = json.Unmarshal(Body, &checkBody)

	if checkBody.Abort != true && checkBody.Parameters == nil {
		workRound.Parameters = []devops.CheckPlayloadParameters{}
		workRound.ID = checkBody.ID
		jsonBody, _ = json.Marshal(workRound)
	} else {
		jsonBody, _ = json.Marshal(checkBody)
	}

	newReqBody = parseBody(bytes.NewBuffer(jsonBody))

	return newReqBody, nil

}

func parseBody(body io.Reader) (newReqBody io.ReadCloser) {
	rc, ok := body.(io.ReadCloser)
	if !ok && body != nil {
		rc = ioutil.NopCloser(body)
	}
	return rc
}
