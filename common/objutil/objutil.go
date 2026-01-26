// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package objutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"reflect"
	"strconv"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	saconfigv1alpha1 "github.com/gardener/scaling-advisor/api/config/v1alpha1"
	sacorev1alpha1 "github.com/gardener/scaling-advisor/api/core/v1alpha1"
	jsonpatch "gopkg.in/evanphx/json-patch.v4"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	apijson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	kjson "k8s.io/apimachinery/pkg/util/json"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/tools/cache"
	sigyaml "sigs.k8s.io/yaml"
)

// ScalingAdvisorScheme is the runtime.Scheme for Scaling Advisor types.
var ScalingAdvisorScheme = runtime.NewScheme()

func init() {
	localSchemeBuilder := runtime.NewSchemeBuilder(
		sacorev1alpha1.AddToScheme,
		saconfigv1alpha1.AddToScheme,
	)
	utilruntime.Must(localSchemeBuilder.AddToScheme(ScalingAdvisorScheme))
}

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

// LoadUsingSchemeIntoRuntimeObject deserializes the object at objPath into the given k8s runtime.Object.
func LoadUsingSchemeIntoRuntimeObject(dirFS fs.FS, objPath string, s *runtime.Scheme, obj runtime.Object) error {
	objDecoder := serializer.NewCodecFactory(s).UniversalDecoder()
	objFile, err := dirFS.Open(objPath)
	if err != nil {
		return err
	}
	objBytes, err := io.ReadAll(objFile)
	if err != nil {
		return err
	}
	if err = runtime.DecodeInto(objDecoder, objBytes, obj); err != nil {
		return err
	}
	return nil
}

// LoadIntoRuntimeObj deserializes the object at objPath into the given k8s runtime.Object using the ScalingAdvisorScheme.
func LoadIntoRuntimeObj(dirFS fs.FS, objPath string, obj runtime.Object) (err error) {
	return LoadUsingSchemeIntoRuntimeObject(dirFS, objPath, ScalingAdvisorScheme, obj)
}

// LoadJSONIntoObject deserializes the JSON object at objPath into the given object using standard json.Unmarshal.
func LoadJSONIntoObject(dirFS fs.FS, objPath string, obj any) (err error) {
	objFile, err := dirFS.Open(objPath)
	if err != nil {
		return err
	}
	objBytes, err := io.ReadAll(objFile)
	if err != nil {
		return err
	}
	return json.Unmarshal(objBytes, obj)
}

// WriteCoreRuntimeObjToYaml marshals the given k8s runtime.Object into YAML and writes it to the specified file path.
func WriteCoreRuntimeObjToYaml(obj runtime.Object, yamlPath string) error {
	data, err := sigyaml.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal object to YAML: %w", err)
	}
	err = os.WriteFile(yamlPath, data, 0600)
	if err != nil {
		return fmt.Errorf("failed to write YAML to %q: %w", yamlPath, err)
	}
	return nil
}

// SetMetaObjectGVK checks if the given object has missing Kind and Version.
// If so, it sets the object's GVK to the gvk passed in the argument.
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

// ResourceListToInt64Map converts the given ResourceList to a map from
// ResourceName to ResourceValue expressed as an int64 number.
func ResourceListToInt64Map(resources corev1.ResourceList) map[corev1.ResourceName]int64 {
	result := make(map[corev1.ResourceName]int64, len(resources))
	for resourceName, quantity := range resources {
		result[resourceName] = quantity.Value()
	}
	return result
}

// Int64MapToResourceList converts the given map from ResourceName to
// ResourceValue(int64) into a ResourceList object.
func Int64MapToResourceList(intMap map[corev1.ResourceName]int64) corev1.ResourceList {
	result := make(corev1.ResourceList, len(intMap))
	for resourceName, intValue := range intMap {
		result[resourceName] = *resource.NewQuantity(intValue, resource.DecimalSI)
	}
	return result
}

// StringKeyValueMapToResourceList converts the given map (resource name string to
// resource quantity string) into a corev1.ResourceList object.
func StringKeyValueMapToResourceList(stringMap map[string]any) corev1.ResourceList {
	result := make(corev1.ResourceList, len(stringMap))
	for resourceName, stringValue := range stringMap {
		result[corev1.ResourceName(resourceName)] = resource.MustParse(stringValue.(string))
	}
	return result
}

// ResourceNameStringValueMapToResourceList converts the given resources map (corev1.ResourceName to human-readable quantity string) into a corev1.ResourceList
func ResourceNameStringValueMapToResourceList(resources map[corev1.ResourceName]string) corev1.ResourceList {
	res := make(corev1.ResourceList, len(resources))
	for n, q := range resources {
		res[n] = resource.MustParse(q)
	}
	return res
}

// IsResourceListEqual compares the given resource lists and checks for equality.
func IsResourceListEqual(r1, r2 corev1.ResourceList) bool {
	for n, q1 := range r1 {
		q2, ok := r2[n]
		if !ok || q1.Cmp(q2) != 0 {
			return false
		}
	}
	for n, q2 := range r2 {
		q1, ok := r1[n]
		if !ok || q1.Cmp(q2) != 0 {
			return false
		}
	}
	return true
}

// SubtractResources subtracts the quantities in b from a. If a resource in b is not found in a, it is ignored.
func SubtractResources(a, b corev1.ResourceList) {
	for res, qty := range b {
		if v, ok := a[res]; ok {
			v.Sub(qty)
			a[res] = v
		}
	}
}

// PatchObject directly patches the given runtime object with the given patchBytes and using the given patch type.
// TODO: Add unit test for this specific objutil method.
func PatchObject(objPtr runtime.Object, name cache.ObjectName, patchType types.PatchType, patchBytes []byte) error {
	objValuePtr := reflect.ValueOf(objPtr)
	if objValuePtr.Kind() != reflect.Pointer || objValuePtr.IsNil() {
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

// PatchObjectStatus patches the given runtime object's status subresource with the given patchBytes.
func PatchObjectStatus(objPtr runtime.Object, objName cache.ObjectName, patch []byte) error {
	objValuePtr := reflect.ValueOf(objPtr)
	if objValuePtr.Kind() != reflect.Pointer || objValuePtr.IsNil() {
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

// SliceOfAnyToRuntimeObj converts a slice of generic objects to a slice of runtime.Objects.
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

// CloneRuntimeObjects creates a cloned slice of the given slice of runtime objects.
func CloneRuntimeObjects(objs []runtime.Object) []runtime.Object {
	result := make([]runtime.Object, 0, len(objs))
	for _, obj := range objs {
		objCopy := obj.DeepCopyObject()
		result = append(result, objCopy)
	}
	return result
}

// SliceOfMetaObjToRuntimeObj converts a slice of metav1.Objects to a slice of runtime.Objects.
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

// ParseObjectResourceVersion parses the resource version of a metav1.Object.
func ParseObjectResourceVersion(obj metav1.Object) (resourceVersion int64, err error) {
	resourceVersion, err = ParseResourceVersion(obj.GetResourceVersion())
	if err != nil {
		err = fmt.Errorf("cannot parse resource version %q for object %q in ns %q: %w", obj.GetResourceVersion(), obj.GetName(), obj.GetNamespace(), err)
	}
	return
}

// ParseResourceVersion parses a string into an int64 representing a resource version.
func ParseResourceVersion(rvStr string) (resourceVersion int64, err error) {
	if rvStr == "" {
		resourceVersion = 0
		return
	}
	resourceVersion, err = strconv.ParseInt(rvStr, 10, 64)
	if err != nil {
		err = fmt.Errorf("cannot parse resource version %q: %w", rvStr, err)
	}
	return
}

// MaxResourceVersion finds the maximum resource version among a list of metav1.Objects.
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

// CacheName returns the cache.ObjectName for a metav1.Object.
func CacheName(mo metav1.Object) cache.ObjectName {
	return cache.NewObjectName(mo.GetNamespace(), mo.GetName())
}

// NamespacedName returns the types.NamespacedName for a metav1.Object.
func NamespacedName(mo metav1.Object) types.NamespacedName {
	return types.NamespacedName{Namespace: mo.GetNamespace(), Name: mo.GetName()}
}

// GetFullNames converts a slice of NamespacedName objects into a slice of their string representations.
func GetFullNames(nsNames []types.NamespacedName) []string {
	names := make([]string, 0, len(nsNames))
	for _, nsName := range nsNames {
		names = append(names, nsName.String())
	}
	return names
}

// GenerateName generates a name by appending a random suffix to the given base name.
func GenerateName(base string) string {
	const suffixLen = 5
	suffix := utilrand.String(suffixLen)
	m := validation.DNS1123SubdomainMaxLength // 253 for subdomains; use DNS1123LabelMaxLength (63) if you need stricter
	if len(base)+len(suffix) > m {
		base = base[:m-len(suffix)]
	}
	return base + suffix
}

// Cast attempts to cast an interface{} into a type T and returns an error if the cast fails.
func Cast[T any](obj any) (t T, err error) {
	t, ok := obj.(T)
	if !ok {
		err = fmt.Errorf("%w: obj has type %T, expected %T", commonerrors.ErrUnexpectedType, obj, TypeName[T]())
	}
	return
}

// TypeName returns the fully qualified name of a type T.
func TypeName[T any]() string {
	var zero T
	typ := reflect.TypeOf(zero)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ.PkgPath() + "." + typ.Name()
}

// AsMeta converts an object to its metav1.Object representation, returning an error if the conversion fails.
func AsMeta(o any) (mo metav1.Object, err error) {
	mo, err = meta.Accessor(o)
	if err != nil {
		err = apierrors.NewInternalError(fmt.Errorf("%w: cannot access meta object for o of type %T", commonerrors.ErrUnexpectedType, o))
	}
	return
}
