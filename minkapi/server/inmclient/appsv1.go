// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package inmclient

import (
	appaccess2 "github.com/gardener/scaling-advisor/minkapi/server/inmclient/access/appaccess"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	"k8s.io/client-go/rest"
)

var (
	_ appsv1.AppsV1Interface = (*appsV1Impl)(nil)
)

type appsV1Impl struct {
	view mkapi.View
}

func (a *appsV1Impl) RESTClient() rest.Interface {
	panic(commonerrors.ErrUnimplemented) //TODO: provide a common implementation of rest.Interface for any resource
}

func (a *appsV1Impl) ControllerRevisions(_ string) appsv1.ControllerRevisionInterface {
	panic(commonerrors.ErrUnimplemented)
}

func (a *appsV1Impl) DaemonSets(_ string) appsv1.DaemonSetInterface {
	panic(commonerrors.ErrUnimplemented)
}

func (a *appsV1Impl) Deployments(_ string) appsv1.DeploymentInterface {
	panic(commonerrors.ErrUnimplemented)
}

func (a *appsV1Impl) ReplicaSets(namespace string) appsv1.ReplicaSetInterface {
	return appaccess2.NewReplicaSetAccess(a.view, namespace)
}

func (a *appsV1Impl) StatefulSets(namespace string) appsv1.StatefulSetInterface {
	return appaccess2.NewStatefulSetAccess(a.view, namespace)
}
