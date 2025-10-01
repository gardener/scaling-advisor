// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package webutil

import (
	"encoding/json"
	"fmt"
	"github.com/go-logr/logr"
	"io"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kjson "k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"net/http"
)

// LoggerMiddleware creates a middleware that injects the logger into the request context.
func LoggerMiddleware(log logr.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Inject the log into the request's context
		ctx := logr.NewContext(r.Context(), log.WithValues("method", r.Method, "requestURI", r.RequestURI))
		// Create a new request with the updated context
		r = r.WithContext(ctx)
		// Call the next handler with the modified request
		next.ServeHTTP(w, r)
	})
}

func ReadBodyIntoObj(w http.ResponseWriter, r *http.Request, obj any) (ok bool) {
	log := logr.FromContextOrDiscard(r.Context())
	data, err := io.ReadAll(r.Body)
	if err != nil {
		HandleBadRequest(w, r, err)
		ok = false
		return
	}
	if err := json.Unmarshal(data, obj); err != nil {
		err = fmt.Errorf("cannot unmarshal JSON for request %q: %w", r.RequestURI, err)
		log.Error(err, "cannot unmarshal JSON for request body", "payload", string(data))
		HandleBadRequest(w, r, err)
		ok = false
		return
	}
	if log.V(4).Enabled() {
		log.V(4).Info("read payload into object", "payload", string(data))
	}
	ok = true
	return
}

func HandleBadRequest(w http.ResponseWriter, r *http.Request, err error) {
	log := logr.FromContextOrDiscard(r.Context())
	err = fmt.Errorf("cannot handle request %q: %w", r.Method+" "+r.RequestURI, err)
	log.Error(err, "bad request", "method", r.Method, "requestURI", r.RequestURI)
	statusErr := apierrors.NewBadRequest(err.Error())
	HandleStatusError(w, r, statusErr)
}

func HandleValidationErrors(w http.ResponseWriter, r *http.Request, errList field.ErrorList) {
	aggregatedErrs := errList.ToAggregate()
	reason := fmt.Sprintf("ScalingAdviceRequest is invalid: %v", aggregatedErrs)
	statusErr := &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Status:  metav1.StatusFailure,
			Code:    http.StatusUnprocessableEntity,
			Reason:  metav1.StatusReasonInvalid,
			Message: reason,
		}}
	HandleStatusError(w, r, statusErr)
}

func HandleConflict(w http.ResponseWriter, r *http.Request, reason string) {
	statusErr := &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Status:  metav1.StatusFailure,
			Code:    http.StatusConflict,
			Reason:  metav1.StatusReasonConflict,
			Message: reason,
		}}
	HandleStatusError(w, r, statusErr)
}

func HandleTooManyRequests(w http.ResponseWriter, r *http.Request, reason string) {
	statusErr := &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Status:  metav1.StatusFailure,
			Code:    http.StatusTooManyRequests,
			Reason:  metav1.StatusReasonTooManyRequests,
			Message: reason,
		}}
	HandleStatusError(w, r, statusErr)
}

func HandleStatusError(w http.ResponseWriter, r *http.Request, statusErr *apierrors.StatusError) {
	log := logr.FromContextOrDiscard(r.Context())
	log.Error(statusErr, "status error", "gvk", statusErr.ErrStatus.GroupVersionKind, "code", statusErr.ErrStatus.Code, "reason", statusErr.ErrStatus.Reason, "message", statusErr.ErrStatus.Message)
	w.WriteHeader(int(statusErr.ErrStatus.Code))
	w.Header().Set("Content-Type", "application/json")
	WriteJsonResponse(w, r, statusErr.ErrStatus)
}

func HandleNotFound(w http.ResponseWriter, r *http.Request, reason string) {
	statusErr := &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Status:  metav1.StatusFailure,
			Code:    http.StatusNotFound,
			Reason:  metav1.StatusReasonNotFound,
			Message: reason,
		}}
	HandleStatusError(w, r, statusErr)
}

func HandleAccepted(w http.ResponseWriter, r *http.Request, message string) {
	statusInProgress := &metav1.Status{
		TypeMeta: metav1.TypeMeta{Kind: "Status"},
		Status:   metav1.StatusSuccess,
		Code:     http.StatusAccepted,
		Message:  message,
	}
	WriteJsonResponse(w, r, statusInProgress)
}

// WriteJsonResponse sets Content-Type to application/json  and encodes the object to the response writer.
func WriteJsonResponse(w http.ResponseWriter, r *http.Request, obj any) {
	log := logr.FromContextOrDiscard(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if err := kjson.NewEncoder(w).Encode(obj); err != nil {
		log.Error(err, "cannot encode response", "obj", obj)
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
	}
}
func HandleInternalServerError(w http.ResponseWriter, r *http.Request, err error) {
	log := logr.FromContextOrDiscard(r.Context())
	statusErr := apierrors.NewInternalError(err)
	log.Error(err, "internal server error")
	w.WriteHeader(http.StatusInternalServerError)
	WriteJsonResponse(w, r, statusErr.ErrStatus)
}
