// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"errors"
	"strings"
	"testing"
)

func TestAsGenerateError(t *testing.T) {
	tests := []struct {
		err           error
		name          string
		id            string
		correlationID string
		expectSubstr  []string
		expectNil     bool
		checkMessage  bool
	}{
		{
			name:          "nil error returns nil",
			id:            "test-id",
			correlationID: "test-corr-id",
			err:           nil,
			expectNil:     true,
		},
		{
			name:          "ErrGenScalingAdvice returns as is",
			id:            "test-id",
			correlationID: "test-corr-id",
			err:           ErrGenScalingAdvice,
			expectNil:     false,
			checkMessage:  true,
			expectSubstr:  []string{"failed to generate scaling advice"},
		},
		{
			name:          "wraps error with request context",
			id:            "request-123",
			correlationID: "corr-456",
			err:           errors.New("some error"),
			expectNil:     false,
			checkMessage:  true,
			expectSubstr:  []string{"request-123", "corr-456", "some error"},
		},
		{
			name:          "wraps another sentinel error",
			id:            "req-789",
			correlationID: "corr-abc",
			err:           ErrNoScalingAdvice,
			expectNil:     false,
			checkMessage:  true,
			expectSubstr:  []string{"req-789", "corr-abc", "no scaling advice"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AsGenerateError(tt.id, tt.correlationID, tt.err)

			if tt.expectNil {
				if result != nil {
					t.Errorf("expected nil error, got %v", result)
				}
				return
			}

			if result == nil {
				t.Error("expected error but got nil")
				return
			}

			if tt.checkMessage {
				errMsg := result.Error()
				for _, substr := range tt.expectSubstr {
					if !strings.Contains(errMsg, substr) {
						t.Errorf("error message %q does not contain expected substring %q", errMsg, substr)
					}
				}
			}

			if tt.err != ErrGenScalingAdvice && !errors.Is(result, ErrGenScalingAdvice) {
				t.Error("wrapped error should be wrapped with ErrGenScalingAdvice")
			}
		})
	}
}
