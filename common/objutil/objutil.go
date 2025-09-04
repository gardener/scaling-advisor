// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package objutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	jsonpatch "gopkg.in/evanphx/json-patch.v4"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kjson "k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/tools/cache"
	"os"
	"reflect"
	sigyaml "sigs.k8s.io/yaml"
	"strconv"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	apijson "k8s.io/apimachinery/pkg/runtime/serializer/json"
)

// ToYAML serializes the given k8s runtime.Object to YAML.
func ToYAML(obj runtime.Object) (string, error) {
	scheme := runtime.NewScheme()
	ser := apijson.NewSerializerWithOptions(apijson.DefaultMetaFactory, scheme, scheme, apijson.SerializerOptions{Yaml: true, Pretty: true})
	var buf bytes.Buffer
	err := ser.Encode(obj, &buf)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// LoadYAMLIntoRuntimeObject deserializes the YAML reading it from the specified path into the given k8s runtime.Object.
func LoadYAMLIntoRuntimeObject(yamlPath string, s *runtime.Scheme, obj runtime.Object) error {
	configDecoder := serializer.NewCodecFactory(s).UniversalDecoder()
	configBytes, err := os.ReadFile(yamlPath)
	if err != nil {
		return err
	}
	if err := runtime.DecodeInto(configDecoder, configBytes, obj); err != nil {
		return err
	}
	return nil
}

// LoadYamlIntoCoreRuntimeObj deserializes the YAML using k8s sig yaml (which has automatic registration for core k8s types) into the given k8s object.
func LoadYamlIntoCoreRuntimeObj(yamlPath string, obj any) (err error) {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		err = fmt.Errorf("failed to read %q: %w", yamlPath, err)
		return
	}
	err = sigyaml.Unmarshal(data, obj)
	if err != nil {
		err = fmt.Errorf("failed to unmarshal object from dataq: %w", err)
	}
	return
}

func SetMetaObjectGVK(obj metav1.Object, gvk schema.GroupVersionKind) {
	if runtimeObj, ok := obj.(runtime.Object); ok {
		objGVK := runtimeObj.GetObjectKind().GroupVersionKind()
		if objGVK.Kind == "" && objGVK.Version == "" {
			runtimeObj.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
				Group:   gvk.Group,
				Version: gvk.Version,
				Kind:    gvk.Kind,
			})
		}
	}
}

func ResourceListToMapInt64(resources corev1.ResourceList) map[corev1.ResourceName]int64 {
	result := make(map[corev1.ResourceName]int64, len(resources))
	for resourceName, quantity := range resources {
		result[resourceName] = quantity.Value()
	}
	return result
}

// PatchObject directly patches the given runtime object with the given patchBytes and using the given patch type.
// TODO: Add unit test for this specific objutil method.
func PatchObject(objPtr runtime.Object, name cache.ObjectName, patchType types.PatchType, patchBytes []byte) error {
	objValuePtr := reflect.ValueOf(objPtr)
	if objValuePtr.Kind() != reflect.Ptr || objValuePtr.IsNil() {
		return fmt.Errorf("object %q must be a non-nil pointer", name)
	}
	objInterface := objValuePtr.Interface()
	originalJSON, err := kjson.Marshal(objInterface)
	if err != nil {
		return fmt.Errorf("failed to marshal object %q: %w", name, err)
	}

	var patchedBytes []byte
	switch patchType {
	case types.StrategicMergePatchType:
		patchedBytes, err = strategicpatch.StrategicMergePatch(originalJSON, patchBytes, objInterface)
		if err != nil {
			return fmt.Errorf("failed to apply strategic merge patch for object %q: %w", name, err)
		}
	case types.MergePatchType:
		patchedBytes, err = jsonpatch.MergePatch(originalJSON, patchBytes)
		if err != nil {
			return fmt.Errorf("failed to apply merge-patch for object %q: %w", name, err)
		}
	default:
		return fmt.Errorf("unsupported patch type %q for object %q", patchType, name)
	}
	err = kjson.Unmarshal(patchedBytes, objInterface)
	if err != nil {
		return fmt.Errorf("failed to unmarshal patched JSON back into obj %q: %w", name, err)
	}
	return nil
}

func PatchObjectStatus(objPtr runtime.Object, objName cache.ObjectName, patch []byte) error {
	objValuePtr := reflect.ValueOf(objPtr)
	if objValuePtr.Kind() != reflect.Ptr || objValuePtr.IsNil() {
		return fmt.Errorf("object %q must be a non-nil pointer", objName)
	}
	statusField := objValuePtr.Elem().FieldByName("Status")
	if !statusField.IsValid() {
		return fmt.Errorf("object %q of type %T has no Status field", objName, objPtr)
	}

	var patchWrapper map[string]json.RawMessage
	err := json.Unmarshal(patch, &patchWrapper)
	if err != nil {
		return fmt.Errorf("failed to parse patch for %q as JSON object: %w", objName, err)
	}
	statusPatchRaw, ok := patchWrapper["status"]
	if !ok {
		return fmt.Errorf("patch for %q does not contain a 'status' objName", objName)
	}

	statusInterface := statusField.Interface()
	originalStatusJSON, err := kjson.Marshal(statusInterface)
	if err != nil {
		return fmt.Errorf("failed to marshal original status for object %q: %w", objName, err)
	}
	patchedStatusJSON, err := strategicpatch.StrategicMergePatch(originalStatusJSON, statusPatchRaw, statusInterface)
	if err != nil {
		return fmt.Errorf("failed to apply strategic merge patch for object %q: %w", objName, err)
	}

	newStatusVal := reflect.New(statusField.Type())
	newStatusPtr := newStatusVal.Interface()
	if err := json.Unmarshal(patchedStatusJSON, newStatusPtr); err != nil {
		return fmt.Errorf("failed to unmarshal patched status for object %q: %w", objName, err)
	}
	statusField.Set(newStatusVal.Elem())
	return nil
}

func SliceOfAnyToRuntimeObj(objs []any) ([]runtime.Object, error) {
	result := make([]runtime.Object, 0, len(objs))
	for _, item := range objs {
		obj, ok := item.(runtime.Object)
		if !ok {
			err := fmt.Errorf("element %T does not implement runtime.Object", item)
			return nil, apierrors.NewInternalError(err)
		}
		result = append(result, obj)
	}
	return result, nil
}
func SliceOfMetaObjToRuntimeObj(objs []metav1.Object) ([]runtime.Object, error) {
	result := make([]runtime.Object, 0, len(objs))
	for _, item := range objs {
		obj, ok := item.(runtime.Object)
		if !ok {
			err := fmt.Errorf("element %T does not implement runtime.Object", item)
			return nil, apierrors.NewInternalError(err)
		}
		result = append(result, obj)
	}
	return result, nil
}

func MaxResourceVersion(objs []metav1.Object) (maxVersion int64, err error) {
	var version int64
	for _, o := range objs {
		version, err = strconv.ParseInt(o.GetResourceVersion(), 10, 64)
		if err != nil {
			err = fmt.Errorf("failed to parse resource version %q from obj %q: %w",
				o.GetResourceVersion(),
				CacheName(o), err)
			return
		}
		if version > maxVersion {
			maxVersion = version
		}
	}
	return
}

func CacheName(mo metav1.Object) cache.ObjectName {
	return cache.NewObjectName(mo.GetNamespace(), mo.GetName())
}
func NamespacedName(mo metav1.Object) types.NamespacedName {
	return types.NamespacedName{Namespace: mo.GetNamespace(), Name: mo.GetName()}
}
