// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package inmclient

import (
	"github.com/gardener/scaling-advisor/minkapi/server/inmclient/access"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	mkapi "github.com/gardener/scaling-advisor/api/minkapi"
	clientresourcev1 "k8s.io/client-go/kubernetes/typed/resource/v1"
	"k8s.io/client-go/rest"
)

var (
	_ clientresourcev1.ResourceV1Interface = (*resourceV1Impl)(nil)
)

type resourceV1Impl struct {
	view mkapi.View
}

func (r *resourceV1Impl) RESTClient() rest.Interface {
	panic(commonerrors.ErrUnimplemented)
}

func (r *resourceV1Impl) DeviceClasses() clientresourcev1.DeviceClassInterface {
	return access.NewDeviceClassAccess(r.view)
}

func (r *resourceV1Impl) ResourceClaims(namespace string) clientresourcev1.ResourceClaimInterface {
	return access.NewResourceClaimAccess(r.view, namespace)
}

func (r *resourceV1Impl) ResourceClaimTemplates(namespace string) clientresourcev1.ResourceClaimTemplateInterface {
	panic(commonerrors.ErrUnimplemented)
}

func (r *resourceV1Impl) ResourceSlices() clientresourcev1.ResourceSliceInterface {
	return access.NewResourceSliceAccess(r.view)
}
