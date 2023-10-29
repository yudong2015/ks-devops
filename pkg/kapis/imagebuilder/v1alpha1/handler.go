/*

  Copyright 2023 The KubeSphere Authors.

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

package v1alpha1

import (
	"context"
	"github.com/emicklei/go-restful"
	//shbuild: shipwright-io/build
	shbuild "github.com/shipwright-io/build/pkg/apis/build/v1alpha1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"kubesphere.io/devops/pkg/apiserver/query"
	devopsClient "kubesphere.io/devops/pkg/client/devops"
	"kubesphere.io/devops/pkg/kapis"
	devopsResource "kubesphere.io/devops/pkg/models/resources/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const LanguageLabelKey = "language"

// apiHandlerOption holds some useful tools for API handler.
type apiHandlerOption struct {
	devopsClient devopsClient.Interface
	client       client.Client
}

// apiHandler contains functions to handle coming request and give a response.
type apiHandler struct {
	apiHandlerOption
}

// newAPIHandler creates an APIHandler.
func newAPIHandler(o apiHandlerOption) *apiHandler {
	return &apiHandler{o}
}

func (h *apiHandler) listImagebuildStrategies(request *restful.Request, response *restful.Response) {
	language := request.QueryParameter("language")
	opt := client.MatchingLabels{
		LanguageLabelKey: language,
	}
	strategyList := &shbuild.ClusterBuildStrategyList{}
	if err := h.client.List(context.Background(), strategyList, opt); err != nil {
		kapis.HandleError(request, response, err)
		return
	}

	queryParam := query.ParseQueryParameter(request)
	apiResult := devopsResource.DefaultList(toBuildStrategyObjects(strategyList.Items),
		queryParam,
		devopsResource.DefaultCompare(),
		devopsResource.DefaultFilter(), nil)

	_ = response.WriteAsJson(apiResult)
}

func toBuildStrategyObjects(apps []shbuild.ClusterBuildStrategy) []runtime.Object {
	objs := make([]runtime.Object, len(apps))
	for i := range apps {
		objs[i] = &apps[i]
	}
	return objs
}

func (h *apiHandler) getImagebuildStrategy(request *restful.Request, response *restful.Response) {
	strategyName := request.PathParameter("imagebuildStrategy")

	// get imagebuildStrategy
	strategy := &shbuild.ClusterBuildStrategy{}
	if err := h.client.Get(context.Background(), client.ObjectKey{Name: strategyName}, strategy); err != nil {
		kapis.HandleError(request, response, err)
		return
	}
	_ = response.WriteEntity(strategy)
}

func (h *apiHandler) listImagebuilds(request *restful.Request, response *restful.Response) {
	namespace := request.PathParameter("namespace")
	queryParam := query.ParseQueryParameter(request)

	opts := make([]client.ListOption, 0, 3)
	opts = append(opts, client.InNamespace(namespace))
	buildList := &shbuild.BuildList{}

	if err := h.client.List(context.Background(), buildList, opts...); err != nil {
		kapis.HandleError(request, response, err)
		return
	}

	apiResult := devopsResource.DefaultList(
		toBuildObjects(buildList.Items),
		queryParam,
		devopsResource.DefaultCompare(),
		devopsResource.DefaultFilter(), nil)

	_ = response.WriteAsJson(apiResult)
}

func toBuildObjects(apps []shbuild.Build) []runtime.Object {
	objs := make([]runtime.Object, len(apps))
	for i := range apps {
		objs[i] = &apps[i]
	}
	return objs
}

func (h *apiHandler) createImagebuild(request *restful.Request, response *restful.Response) {
	build := shbuild.Build{}
	err := request.ReadEntity(&build)
	if err != nil {
		klog.Error(err)
		kapis.HandleBadRequest(response, request, err)
		return
	}

	if err := h.client.Create(context.Background(), &build); err != nil {
		kapis.HandleError(request, response, err)
		return
	}
	_ = response.WriteEntity(build)
}

func (h *apiHandler) updateImagebuild(request *restful.Request, response *restful.Response) {
	namespace := request.PathParameter("namespace")
	imagebuild := request.PathParameter("imagebuild")

	oldBuild := shbuild.Build{}
	if err := h.client.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: imagebuild}, &oldBuild); err != nil {
		kapis.HandleError(request, response, err)
		return
	}

	err := request.ReadEntity(&oldBuild)
	if err != nil {
		klog.Error(err)
		kapis.HandleBadRequest(response, request, err)
		return
	}

	if err := h.client.Update(context.Background(), &oldBuild); err != nil {
		kapis.HandleError(request, response, err)
		return
	}

	_ = response.WriteEntity(oldBuild)
}

func (h *apiHandler) getImagebuild(request *restful.Request, response *restful.Response) {
	namespace := request.PathParameter("namespace")
	imagebuild := request.PathParameter("imagebuild")

	build := shbuild.Build{}
	if err := h.client.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: imagebuild}, &build); err != nil {
		kapis.HandleError(request, response, err)
		return
	}
	_ = response.WriteEntity(&build)
}

func (h *apiHandler) deleteImagebuild(request *restful.Request, response *restful.Response) {
	namespace := request.PathParameter("namespace")
	imagebuild := request.PathParameter("imagebuild")

	// get imagebuild
	build := shbuild.Build{}
	if err := h.client.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: imagebuild}, &build); err != nil {
		kapis.HandleError(request, response, err)
		return
	}
	if err := h.client.Delete(context.Background(), &build); err != nil {
		kapis.HandleError(request, response, err)
		return
	}
	_ = response.WriteEntity(&build)
}

func (h *apiHandler) createImagebuildRun(request *restful.Request, response *restful.Response) {
	namespace := request.PathParameter("namespace")
	buildrunName := request.PathParameter("imagebuildrun")
	imagebuild := request.QueryParameter("imagebuild")

	buildRun := shbuild.BuildRun{}
	err := request.ReadEntity(&buildRun)
	if err != nil {
		klog.Error(err)
		kapis.HandleBadRequest(response, request, err)
		return
	}

	buildRun.ObjectMeta.GenerateName = buildrunName + "-"
	buildRun.Spec.BuildRef.Name = imagebuild
	buildRun.Namespace = namespace

	if err := h.client.Create(context.Background(), &buildRun); err != nil {
		kapis.HandleError(request, response, err)
		return
	}

	_ = response.WriteEntity(buildRun)
}

func (h *apiHandler) getImagebuildRun(request *restful.Request, response *restful.Response) {
	namespace := request.PathParameter("namespace")
	buildrunName := request.PathParameter("imagebuildrun")

	// get imagebuildRun
	buildRun := shbuild.BuildRun{}
	if err := h.client.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: buildrunName}, &buildRun); err != nil {
		kapis.HandleError(request, response, err)
		return
	}
	_ = response.WriteEntity(&buildRun)
}

func (h *apiHandler) deleteImagebuildRun(request *restful.Request, response *restful.Response) {
	namespace := request.PathParameter("namespace")
	buildrunName := request.PathParameter("imagebuildrun")
	ctx := context.Background()

	// get imagebuild
	buildRun := shbuild.BuildRun{}
	if err := h.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: buildrunName}, &buildRun); err != nil {
		kapis.HandleError(request, response, err)
		return
	}
	if err := h.client.Delete(context.Background(), &buildRun); err != nil {
		kapis.HandleError(request, response, err)
		return
	}
	_ = response.WriteEntity(&buildRun)
}

func (h *apiHandler) listImagebuildRuns(request *restful.Request, response *restful.Response) {
	namespace := request.PathParameter("namespace")
	imagebuild := request.PathParameter("imagebuild")

	queryParam := query.ParseQueryParameter(request)
	labelSelector := labels.SelectorFromSet(labels.Set{"build.shipwright.io/name": imagebuild})

	opts := make([]client.ListOption, 0, 3)
	opts = append(opts, client.InNamespace(namespace))
	opts = append(opts, client.MatchingLabelsSelector{Selector: labelSelector})

	buildRunList := &shbuild.BuildRunList{}
	// fetch PipelineRuns
	if err := h.client.List(context.Background(), buildRunList, opts...); err != nil {
		kapis.HandleError(request, response, err)
		return
	}

	apiResult := devopsResource.DefaultList(toBuildRunObjects(buildRunList.Items),
		queryParam,
		devopsResource.DefaultCompare(),
		devopsResource.DefaultFilter(), nil)

	_ = response.WriteAsJson(apiResult)
}

func toBuildRunObjects(apps []shbuild.BuildRun) []runtime.Object {
	objs := make([]runtime.Object, len(apps))
	for i := range apps {
		objs[i] = &apps[i]
	}
	return objs
}
