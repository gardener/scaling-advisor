// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package inmclient

import (
	"errors"
	"github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/gardener/scaling-advisor/minkapi/view/inmclient/access/appaccess"

	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	"k8s.io/client-go/rest"
)

var (
	_ appsv1.AppsV1Interface = (*appsV1Impl)(nil)
)

type appsV1Impl struct {
	view api.View
}

func (a *appsV1Impl) RESTClient() rest.Interface {
	panic(errors.ErrUnsupported) //TODO: provide a common implementation of rest.Interface for any resource
}

func (a *appsV1Impl) ControllerRevisions(_ string) appsv1.ControllerRevisionInterface {
	panic(errors.ErrUnsupported)
}

func (a *appsV1Impl) DaemonSets(_ string) appsv1.DaemonSetInterface {
	panic(errors.ErrUnsupported)
}

func (a *appsV1Impl) Deployments(_ string) appsv1.DeploymentInterface {
	panic(errors.ErrUnsupported)
}

func (a *appsV1Impl) ReplicaSets(namespace string) appsv1.ReplicaSetInterface {
	return appaccess.NewReplicaSetAccess(a.view, namespace)
}

func (a *appsV1Impl) StatefulSets(namespace string) appsv1.StatefulSetInterface {
	return appaccess.NewStatefulSetAccess(a.view, namespace)
}
