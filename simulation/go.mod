module github.com/gardener/scaling-advisor/simulation

go 1.26.3

require (
	github.com/gardener/scaling-advisor/api v0.0.0
	github.com/gardener/scaling-advisor/minkapi v0.0.0
	github.com/gardener/scaling-advisor/simulation/api v0.0.0
	github.com/gardener/scaling-advisor/common v0.0.0
	github.com/go-logr/logr v1.4.3
	github.com/google/go-cmp v0.7.0
	golang.org/x/sync v0.18.0
	k8s.io/api v0.34.4
	k8s.io/apimachinery v0.34.4
	k8s.io/client-go v0.34.4
	k8s.io/component-base v0.34.3
	k8s.io/klog/v2 v2.130.1
	k8s.io/kubernetes v1.35.2
)


replace (
	github.com/gardener/scaling-advisor/api => ../api
	github.com/gardener/scaling-advisor/minkapi => ../minkapi
	github.com/gardener/scaling-advisor/simulation/api => ./api
	github.com/gardener/scaling-advisor/common => ../common
)

// NOTE: Primarily needed for Goland/Intelij Go plugin to work correctly, not for the gopls or go compiler
replace (
	k8s.io/api => k8s.io/api v0.34.3
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.34.3
	k8s.io/apimachinery => k8s.io/apimachinery v0.34.3
	k8s.io/apiserver => k8s.io/apiserver v0.34.3
	k8s.io/client-go => k8s.io/client-go v0.34.3
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.34.3
	k8s.io/component-base => k8s.io/component-base v0.34.4
	k8s.io/component-helpers => k8s.io/component-helpers v0.34.3
	k8s.io/controller-manager => k8s.io/controller-manager v0.34.3
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.34.3
	k8s.io/dynamic-resource-allocation => k8s.io/dynamic-resource-allocation v0.34.3
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20250710124328-f3f2b991d03b
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.34.3
	k8s.io/kubelet => k8s.io/kubelet v0.34.3
	k8s.io/kubernetes => k8s.io/kubernetes v1.34.3
)
