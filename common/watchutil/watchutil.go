// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package watchutil

import (
	"context"
	"k8s.io/apimachinery/pkg/watch"
)

// CombineTwoWatchers merges exactly two watchers into one ProxyWatcher.
// The returned watcher will close when both inputs close, when ctx is canceled,
// or when Stop() is called on the returned watcher.
// TODO: add unit test for me
func CombineTwoWatchers(ctx context.Context, w1, w2 watch.Interface) watch.Interface {
	out := make(chan watch.Event)
	proxy := watch.NewProxyWatcher(out)

	go func() {
		defer close(out) // signals end of events to proxy

		c1, c2 := w1.ResultChan(), w2.ResultChan()
		var open1, open2 = true, true

		for open1 || open2 {
			select {
			case e, ok := <-c1:
				if !ok {
					open1 = false
					c1 = nil // disable this case
					continue
				}
				select {
				case out <- e:
				case <-ctx.Done():
					return
				case <-proxy.StopChan():
					return
				}
			case e, ok := <-c2:
				if !ok {
					open2 = false
					c2 = nil
					continue
				}
				select {
				case out <- e:
				case <-ctx.Done():
					return
				case <-proxy.StopChan():
					return
				}
			case <-ctx.Done():
				return
			case <-proxy.StopChan():
				return
			}
		}
	}()

	return proxy
}
