// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package inmclient

import (
	"github.com/gardener/scaling-advisor/minkapi/server/inmclient/access"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	clientpolicyv1 "k8s.io/client-go/kubernetes/typed/policy/v1"
	"k8s.io/client-go/rest"
)

var (
	_ clientpolicyv1.PolicyV1Interface = (*policyV1Impl)(nil)
)

type policyV1Impl struct {
	view mkapi.View
}

func (p *policyV1Impl) RESTClient() rest.Interface {
	panic(commonerrors.ErrUnimplemented)
}

func (p *policyV1Impl) Evictions(namespace string) clientpolicyv1.EvictionInterface {
	panic(commonerrors.ErrUnimplemented)
}

func (p *policyV1Impl) PodDisruptionBudgets(namespace string) clientpolicyv1.PodDisruptionBudgetInterface {
	return access.NewPodDisruptionBudgetAccess(p.view, namespace)
}
