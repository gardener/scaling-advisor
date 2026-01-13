// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ioutil

import "io"

// CloserFunc is a function type that implements io.Closer.
type CloserFunc func() error

// Close releases resources associated with the CloserFunc implementation by invoking the function it wraps.
func (f CloserFunc) Close() error {
	return f()
}

// CloseQuietly safely closes an io.Closer, ignoring and suppressing any error during the close operation.
func CloseQuietly(closer io.Closer) {
	if closer != nil {
		_ = closer.Close()
	}
}
