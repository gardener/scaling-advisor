// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package clientutil

import (
	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/go-logr/logr"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"net/http"
	"time"
)

// BuildClients currently constructs static and dynamic client-go clients given a kubeconfig file.
func BuildClients(log logr.Logger, kubeConfigPath string) (client kubernetes.Interface, dynClient dynamic.Interface, err error) {
	clientConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return
	}
	clientConfig.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return &loggingRoundTripper{
			rt:  rt,
			log: log,
		}
	}
	clientConfig.ContentType = "application/json"
	client, err = kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return
	}
	dynClient, err = dynamic.NewForConfig(clientConfig)
	if err != nil {
		return
	}
	return
}

func BuildInformerFactories(client kubernetes.Interface, dyncClient dynamic.Interface, resyncPeriod time.Duration) (informerFactory informers.SharedInformerFactory, dynInformerFactory dynamicinformer.DynamicSharedInformerFactory) {
	informerFactory = informers.NewSharedInformerFactory(client, resyncPeriod)
	dynInformerFactory = dynamicinformer.NewDynamicSharedInformerFactory(dyncClient, resyncPeriod)
	return
}

func CreateNetworkClientFacades(log logr.Logger, kubeConfigPath string, resyncPeriod time.Duration) (clientFacades commontypes.ClientFacades, err error) {
	client, dynClient, err := BuildClients(log, kubeConfigPath)
	if err != nil {
		return
	}
	informerFactory, dynInformerFactory := BuildInformerFactories(client, dynClient, resyncPeriod)
	clientFacades = commontypes.ClientFacades{
		Mode:               commontypes.ClientAccessNetwork,
		Client:             client,
		DynClient:          dynClient,
		InformerFactory:    informerFactory,
		DynInformerFactory: dynInformerFactory,
	}
	return
}

type loggingRoundTripper struct {
	rt  http.RoundTripper
	log logr.Logger
}

func (l *loggingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Log the outgoing request
	l.log.Info("outgoing request", "method", req.Method, "url", req.URL)

	// Perform the request
	resp, err := l.rt.RoundTrip(req)

	// Log the response
	if err != nil {
		l.log.Error(err, "failed request", "method", req.Method, "url", req.URL)
	} else {
		l.log.Info("performed roundtrip", "method", req.Method, "url", req.URL, "statusLine", resp.Status)
	}
	return resp, err
}
