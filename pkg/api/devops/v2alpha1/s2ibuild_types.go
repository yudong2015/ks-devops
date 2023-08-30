package v2alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	//"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"kubesphere.io/devops/pkg/api/devops"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// Build is the Schema representing a Build definition
// +kubebuilder:unservedversion
// +kubebuilder:resource:path=builds,scope=Namespaced
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.reason",description="The reason of the registered Build, either an error or succeed message"
// +kubebuilder:printcolumn:name="BuildStrategyKind",type="string",JSONPath=".spec.strategy.kind",description="The BuildStrategy type which is used for this Build"
// +kubebuilder:printcolumn:name="BuildStrategyName",type="string",JSONPath=".spec.strategy.name",description="The BuildStrategy name which is used for this Build"
// +kubebuilder:printcolumn:name="CreationTime",type="date",JSONPath=".metadata.creationTimestamp",description="The create time of this Build"
type Build struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BuildSpec `json:"spec,omitempty"`
}

func (b Build) DeepCopyObject() runtime.Object {
	//TODO implement me
	panic("implement me")
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BuildList contains a list of Build
type BuildList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Build `json:"items"`
}

func (b BuildList) DeepCopyObject() runtime.Object {
	//TODO implement me
	panic("implement me")
}

type BuildSpec struct {
	Source   Source     `json:"source"`
	Strategy Strategy   `json:"strategy"`
	Output   OutputSpec `json:"output,omitempty"`
}

// StrategyName returns the name of the configured strategy, or 'undefined' in
// case the strategy is nil (not set)
func (buildSpec *BuildSpec) StrategyName() string {
	if buildSpec == nil {
		return "undefined (nil buildSpec)"
	}

	return buildSpec.Strategy.Name
}

type Source struct {
	URL        *string `json:"url,omitempty"`
	ContextDir *string `json:"contextDir,omitempty"`
}

type Strategy struct {
	Name string `json:"name,omitempty"`
	Kind string `json:"kind,omitempty"`
}

type OutputSpec struct {
	Image       string          `json:"image,omitempty"`
	Credentials CredentialsSpec `json:"credentials,omitempty"`
}

type CredentialsSpec struct {
	Name string `json:"name,omitempty"`
}

var (
	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: devops.GroupName, Version: "v2alpha1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

// Resource is required by pkg/client/listers/...
func Resource(resource string) schema.GroupResource {
	return GroupVersion.WithResource(resource).GroupResource()
}

func init() {
	SchemeBuilder.Register(&Build{}, &BuildList{})
}
