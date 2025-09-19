// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package inmclient

import (
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
	//TODO implement me
	panic("implement me")
}

func (a *appsV1Impl) ControllerRevisions(namespace string) appsv1.ControllerRevisionInterface {
	//TODO implement me
	panic("implement me")
}

func (a *appsV1Impl) DaemonSets(namespace string) appsv1.DaemonSetInterface {
	//TODO implement me
	panic("implement me")
}

func (a *appsV1Impl) Deployments(namespace string) appsv1.DeploymentInterface {
	//TODO implement me
	panic("implement me")
}

func (a *appsV1Impl) ReplicaSets(namespace string) appsv1.ReplicaSetInterface {
	return &replicaSetAccessImpl{
		resourceAccessImpl: resourceAccessImpl{
			view:      a.view,
			namespace: namespace,
		},
	}
}

func (a appsV1Impl) StatefulSets(namespace string) appsv1.StatefulSetInterface {
	//TODO implement me
	panic("implement me")
}
