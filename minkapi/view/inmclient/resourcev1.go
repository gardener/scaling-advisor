// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package inmclient

import (
	"errors"

	"github.com/gardener/scaling-advisor/minkapi/api"
	"github.com/gardener/scaling-advisor/minkapi/view/inmclient/access/resourceaccess"

	clientresourcev1 "k8s.io/client-go/kubernetes/typed/resource/v1"
	"k8s.io/client-go/rest"
)

var (
	_ clientresourcev1.ResourceV1Interface = (*resourceV1Impl)(nil)
)

type resourceV1Impl struct {
	view api.View
}

func (r *resourceV1Impl) RESTClient() rest.Interface {
	panic(errors.ErrUnsupported)
}

func (r *resourceV1Impl) DeviceClasses() clientresourcev1.DeviceClassInterface {
	return resourceaccess.NewDeviceClassAccess(r.view)
}

func (r *resourceV1Impl) ResourceClaims(namespace string) clientresourcev1.ResourceClaimInterface {
	return resourceaccess.NewResourceClaimAccess(r.view, namespace)
}

func (r *resourceV1Impl) ResourceClaimTemplates(_ string) clientresourcev1.ResourceClaimTemplateInterface {
	panic(errors.ErrUnsupported)
}

func (r *resourceV1Impl) ResourceSlices() clientresourcev1.ResourceSliceInterface {
	return resourceaccess.NewResourceSliceAccess(r.view)
}
