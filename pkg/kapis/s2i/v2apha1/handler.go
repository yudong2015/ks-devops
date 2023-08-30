package v2alpha1

import (
	"context"
	"github.com/emicklei/go-restful"
	"github.com/shipwright-io/build/pkg/apis/build/v1alpha1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"kubesphere.io/devops/pkg/apiserver/query"
	devopsClient "kubesphere.io/devops/pkg/client/devops"
	"kubesphere.io/devops/pkg/kapis"
	resourcesV1alpha3 "kubesphere.io/devops/pkg/models/resources/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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

func (h *apiHandler) listImageBuilds(request *restful.Request, response *restful.Response) {
	nsName := request.PathParameter("namespace")
	//buildName := request.PathParameter("imageBuild")

	queryParam := query.ParseQueryParameter(request)

	//labelSelector := labels.SelectorFromSet(labels.Set{"build.shipwright.io/name": buildName})

	opts := make([]client.ListOption, 0, 3)
	opts = append(opts, client.InNamespace(nsName))
	//opts = append(opts, client.MatchingLabelsSelector{Selector: labelSelector})

	buildList := &v1alpha1.BuildList{}
	// fetch PipelineRuns
	if err := h.client.List(context.Background(), buildList, opts...); err != nil {
		kapis.HandleError(request, response, err)
		return
	}

	apiResult := resourcesV1alpha3.DefaultList(toBuildObjects(buildList.Items), queryParam, resourcesV1alpha3.DefaultCompare(), resourcesV1alpha3.DefaultFilter(), nil)

	_ = response.WriteAsJson(apiResult)
}

func toBuildObjects(apps []v1alpha1.Build) []runtime.Object {
	objs := make([]runtime.Object, len(apps))
	for i := range apps {
		objs[i] = &apps[i]
	}
	return objs
}

func (h *apiHandler) createImageBuild(request *restful.Request, response *restful.Response) {
	nsName := request.PathParameter("namespace")
	imageBuildName := request.PathParameter("imageBuild")
	codeUrl := request.QueryParameter("codeUrl")
	languageKind := request.QueryParameter("languageKind")
	outputImageUrl := request.QueryParameter("outputImageUrl")

	build := v1alpha1.Build{}
	err := request.ReadEntity(&build)
	if err != nil {
		klog.Error(err)
		kapis.HandleBadRequest(response, request, err)
		return
	}

	build.Namespace = nsName
	build.Name = imageBuildName + "-"
	build.Spec.Source.URL = &codeUrl
	if "nodejs" == languageKind {
		build.Spec.Strategy.Name = "buildpacks-v3"
	}
	//build.Spec.Strategy.Name = "buildpacks-v3"
	build.Spec.Output.Image = outputImageUrl

	if err := h.client.Create(context.Background(), &build); err != nil {
		kapis.HandleError(request, response, err)
		return
	}

	_ = response.WriteEntity(build)
}

func (h *apiHandler) updateImageBuild(request *restful.Request, response *restful.Response) {
	nsName := request.PathParameter("namespace")
	imageBuildName := request.PathParameter("imageBuild")

	//获取旧build
	oldBuild := v1alpha1.Build{}
	if err := h.client.Get(context.Background(), client.ObjectKey{Name: imageBuildName}, &oldBuild); err != nil {
		kapis.HandleError(request, response, err)
		return
	}

	codeUrl := request.QueryParameter("codeUrl")
	languageKind := request.QueryParameter("languageKind")
	outputImageUrl := request.QueryParameter("outputImageUrl")

	err := request.ReadEntity(&oldBuild)
	if err != nil {
		klog.Error(err)
		kapis.HandleBadRequest(response, request, err)
		return
	}

	//oldBuild.Name = imageBuildName + "-"
	oldBuild.Spec.Source.URL = &codeUrl
	if "nodejs" == languageKind {
		oldBuild.Spec.Strategy.Name = "buildpacks-v3"
	}
	//build.Spec.Strategy.Name = "buildpacks-v3"
	oldBuild.Spec.Output.Image = outputImageUrl
	oldBuild.Namespace = nsName

	if err := h.client.Update(context.Background(), &oldBuild); err != nil {
		kapis.HandleError(request, response, err)
		return
	}

	_ = response.WriteEntity(oldBuild)
}

func (h *apiHandler) getImageBuild(request *restful.Request, response *restful.Response) {
	nsName := request.PathParameter("namespace")
	imageBuildName := request.PathParameter("ImageBuild")

	// get imageBuild
	build := v1alpha1.Build{}
	if err := h.client.Get(context.Background(), client.ObjectKey{Namespace: nsName, Name: imageBuildName}, &build); err != nil {
		kapis.HandleError(request, response, err)
		return
	}
	_ = response.WriteEntity(&build)
}

func (h *apiHandler) deleteImageBuild(request *restful.Request, response *restful.Response) {
	nsName := request.PathParameter("namespace")
	imageBuildName := request.PathParameter("ImageBuild")

	// get imageBuild
	build := v1alpha1.Build{}
	if err := h.client.Get(context.Background(), client.ObjectKey{Namespace: nsName, Name: imageBuildName}, &build); err != nil {
		kapis.HandleError(request, response, err)
		return
	}
	if err := h.client.Delete(context.Background(), &build); err != nil {
		kapis.HandleError(request, response, err)
		return
	}
	_ = response.WriteEntity(&build)
}

func (h *apiHandler) createImageBuildRun(request *restful.Request, response *restful.Response) {
	nsName := request.PathParameter("namespace")
	imageBuildRunName := request.PathParameter("imageBuildRun")
	buildName := request.QueryParameter("ImageBuild")

	buildRun := v1alpha1.BuildRun{}
	err := request.ReadEntity(&buildRun)
	if err != nil {
		klog.Error(err)
		kapis.HandleBadRequest(response, request, err)
		return
	}

	buildRun.ObjectMeta.GenerateName = imageBuildRunName + "-"
	buildRun.Spec.BuildRef.Name = buildName
	buildRun.Namespace = nsName

	if err := h.client.Create(context.Background(), &buildRun); err != nil {
		kapis.HandleError(request, response, err)
		return
	}

	_ = response.WriteEntity(buildRun)
}

func (h *apiHandler) deleteImageBuildRun(request *restful.Request, response *restful.Response) {
	nsName := request.PathParameter("namespace")
	imageBuildRunName := request.PathParameter("ImageBuildRun")
	ctx := context.Background()

	// get imageBuild
	buildRun := v1alpha1.BuildRun{}
	if err := h.client.Get(ctx, client.ObjectKey{Namespace: nsName, Name: imageBuildRunName}, &buildRun); err != nil {
		kapis.HandleError(request, response, err)
		return
	}
	if err := h.client.Delete(context.Background(), &buildRun); err != nil {
		kapis.HandleError(request, response, err)
		return
	}
	_ = response.WriteEntity(&buildRun)
}

//func (h *handler) deleteGitRepositories(req *restful.Request, res *restful.Response) {
//	namespace := common.GetPathParameter(req, common.NamespacePathParameter)
//	repoName := common.GetPathParameter(req, pathParameterGitRepository)
//	ctx := context.Background()
//
//	repo := &v1alpha3.GitRepository{}
//	err := h.Get(ctx, types.NamespacedName{
//		Namespace: namespace,
//		Name:      repoName,
//	}, repo)
//	if err == nil {
//		err = h.Delete(ctx, repo)
//	}
//	common.Response(req, res, repo, err)
//}

func (h *apiHandler) listImageBuildRuns(request *restful.Request, response *restful.Response) {
	nsName := request.PathParameter("namespace")
	buildName := request.PathParameter("ImageBuild")

	queryParam := query.ParseQueryParameter(request)
	labelSelector := labels.SelectorFromSet(labels.Set{"build.shipwright.io/name": buildName})

	opts := make([]client.ListOption, 0, 3)
	opts = append(opts, client.InNamespace(nsName))
	opts = append(opts, client.MatchingLabelsSelector{Selector: labelSelector})

	buildRunList := &v1alpha1.BuildRunList{}
	// fetch PipelineRuns
	if err := h.client.List(context.Background(), buildRunList, opts...); err != nil {
		kapis.HandleError(request, response, err)
		return
	}

	apiResult := resourcesV1alpha3.DefaultList(toBuildRunObjects(buildRunList.Items), queryParam, resourcesV1alpha3.DefaultCompare(), resourcesV1alpha3.DefaultFilter(), nil)

	_ = response.WriteAsJson(apiResult)
}

func toBuildRunObjects(apps []v1alpha1.BuildRun) []runtime.Object {
	objs := make([]runtime.Object, len(apps))
	for i := range apps {
		objs[i] = &apps[i]
	}
	return objs
}

func (h *apiHandler) getImageBuildStrategy(request *restful.Request, response *restful.Response) {
	nsName := request.PathParameter("namespace")
	imageBuildStrategyName := request.PathParameter("imageBuildStrategy")

	// get imageBuild
	Strategy := v1alpha1.BuildStrategy{}
	if err := h.client.Get(context.Background(), client.ObjectKey{Namespace: nsName, Name: imageBuildStrategyName}, &Strategy); err != nil {
		kapis.HandleError(request, response, err)
		return
	}
	_ = response.WriteEntity(&Strategy)
}

func (h *apiHandler) listImageBuildStrategy(request *restful.Request, response *restful.Response) {
	nsName := request.PathParameter("namespace")

	queryParam := query.ParseQueryParameter(request)

	//labelSelector := labels.SelectorFromSet(labels.Set{"build.shipwright.io/name": buildName})

	opts := make([]client.ListOption, 0, 3)
	opts = append(opts, client.InNamespace(nsName))
	//opts = append(opts, client.MatchingLabelsSelector{Selector: labelSelector})

	buildStrategyList := &v1alpha1.BuildStrategyList{}
	// fetch PipelineRuns
	if err := h.client.List(context.Background(), buildStrategyList, opts...); err != nil {
		kapis.HandleError(request, response, err)
		return
	}

	apiResult := resourcesV1alpha3.DefaultList(toBuildStrategyObjects(buildStrategyList.Items), queryParam, resourcesV1alpha3.DefaultCompare(), resourcesV1alpha3.DefaultFilter(), nil)

	_ = response.WriteAsJson(apiResult)
}

func toBuildStrategyObjects(apps []v1alpha1.BuildStrategy) []runtime.Object {
	objs := make([]runtime.Object, len(apps))
	for i := range apps {
		objs[i] = &apps[i]
	}
	return objs
}
