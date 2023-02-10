/*
Copyright 2022 The KubeSphere Authors.

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

package webhook

import (
	"github.com/emicklei/go-restful"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"kubesphere.io/devops/pkg/event/common"
	"kubesphere.io/devops/pkg/event/workflowrun"
	"kubesphere.io/devops/pkg/kapis"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Handler handles requests from webhooks.
type Handler struct {
	client.Client
}

// NewHandler creates a new handler for handling webhooks.
func NewHandler(genericClient client.Client) *Handler {
	return &Handler{
		Client: genericClient,
	}
}

// ReceiveEventsFromJenkins receives events from Jenkins
func (handler *Handler) ReceiveEventsFromJenkins(request *restful.Request, response *restful.Response) {
	// concrete event body
	event := &common.Event{}
	klog.Info("### receive event ..")
	if err := request.ReadEntity(event); err != nil {
		klog.Info("### parse event error: ", err)
		kapis.HandleError(request, response, err)
		return
	}
	klog.Infof("### event ID: %s, source: %s, type: %s, dataType: %s, time: %s", event.ID, event.Source, event.Type, event.DataType, event.Time)
	klog.Infof("### event data: %s", string(event.Data))

	// TODO Make all handlers execute asynchronously

	// register WorkflowRun event handler
	var errs []error
	workflowRunHandlers := workflowrun.Handlers{
		HandleInitialize: handler.handleWorkflowRunInitialize,
		// TODO Handler others
		HandleStarted:   nil,
		HandleFinalized: nil,
		HandleCompleted: nil,
		HandleDeleted:   nil,
	}
	if err := workflowRunHandlers.Handle(event); err != nil {
		errs = append(errs, err)
	}

	// TODO Register other event handlers here

	if len(errs) > 0 {
		kapis.HandleError(request, response, errors.NewAggregate(errs))
	}
}
