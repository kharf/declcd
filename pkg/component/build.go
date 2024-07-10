// Copyright 2024 kharf
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package component

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	internalCue "github.com/kharf/declcd/internal/cue"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/kube"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type MetadataNode = kube.ManifestMetadataNode
type Manifest = kube.Manifest
type ExtendedUnstructured = kube.ExtendedUnstructured
type AttributeInfo = kube.ManifestAttributeInfo
type FieldMetadata = kube.ManifestFieldMetadata

var (
	ErrMissingField = errors.New("Missing content field")
)

const (
	ignoreAttr = "ignore"
)

// Builder compiles and decodes CUE kubernetes manifest definitions of a component to the corresponding Go struct.
type Builder struct {
}

// NewBuilder contructs a [Builder].
func NewBuilder() Builder {
	return Builder{}
}

// BuildOptions defining which package is compiled and how it is done.
type BuildOptions struct {
	packagePath string
	projectRoot string
}

type buildOptions = func(opts *BuildOptions)

// WithPackagePath provides package path configuration.
func WithPackagePath(packagePath string) buildOptions {
	return func(opts *BuildOptions) {
		opts.packagePath = packagePath
	}
}

// WithProjectRoot provides the path to the project root.
func WithProjectRoot(projectRootPath string) buildOptions {
	return func(opts *BuildOptions) {
		opts.projectRoot = projectRootPath
	}
}

const (
	ProjectRootPath = "."
)

// Build accepts options defining which cue package to compile
// and compiles it to a slice of component Instances.
func (b Builder) Build(opts ...buildOptions) ([]Instance, error) {
	options := &BuildOptions{
		packagePath: "",
		projectRoot: ProjectRootPath,
	}
	for _, opt := range opts {
		opt(options)
	}

	value, err := internalCue.BuildPackage(
		options.packagePath,
		options.projectRoot,
	)
	if err != nil {
		return nil, err
	}

	iter, err := value.Fields()
	if err != nil {
		return nil, err
	}

	instances := make([]Instance, 0)
	for iter.Next() {
		componentValue := iter.Value()

		instanceType, err := getStringValue(componentValue, "type")
		if err != nil {
			return nil, err
		}

		id, err := getStringValue(componentValue, "id")
		if err != nil {
			return nil, err
		}

		dependencies, err := getStringSliceValue(componentValue, "dependencies")
		if err != nil {
			return nil, err
		}

		switch instanceType {
		case "Manifest":
			contentValue, err := getValue(componentValue, "content")
			if err != nil {
				return nil, err
			}

			metadata := MetadataNode{}
			content := make(map[string]any, 1)
			attrInfo, err := decodeValue(*contentValue, content, metadata, true)
			if err != nil {
				return nil, err
			}

			contentField := content["content"]
			manifest := Manifest{
				ID:           id,
				Dependencies: dependencies,
				Content: ExtendedUnstructured{
					Unstructured: &unstructured.Unstructured{
						Object: contentField.(map[string]any),
					},
					AttributeInfo: *attrInfo,
				},
			}

			if attrInfo.HasIgnoreConflictAttributes {
				manifest.Content.Metadata = metadata["content"]
			}

			if err := validateManifest(manifest); err != nil {
				return nil, err
			}
			instances = append(instances, &manifest)

		case "HelmRelease":
			name, err := getStringValue(componentValue, "name")
			if err != nil {
				return nil, err
			}

			namespace, err := getStringValue(componentValue, "namespace")
			if err != nil {
				return nil, err
			}

			chart, err := decodeChart(componentValue)
			if err != nil {
				return nil, err
			}

			values, err := decodeValues(componentValue)
			if err != nil {
				return nil, err
			}

			patchesValue, err := getValue(componentValue, "patches")
			if err != nil {
				return nil, err
			}

			patchesValueIter, err := patchesValue.List()
			if err != nil {
				return nil, err
			}

			patches := helm.NewPatches()
			for patchesValueIter.Next() {
				metadata := MetadataNode{}
				content := make(map[string]any)
				value := patchesValueIter.Value()
				attrInfo, err := decodeValue(value, content, metadata, true)
				if err != nil {
					return nil, err
				}

				unstr := kube.ExtendedUnstructured{
					Unstructured: &unstructured.Unstructured{
						Object: content,
					},
					AttributeInfo: *attrInfo,
				}
				if attrInfo.HasIgnoreConflictAttributes {
					unstr.Metadata = &metadata
				}

				patches.Put(unstr)
			}

			crdsValue, err := getValue(componentValue, "crds")
			if err != nil {
				return nil, err
			}

			allowUpgrade, err := getBoolValue(*crdsValue, "allowUpgrade")
			if err != nil {
				return nil, err
			}

			hr := &helm.ReleaseComponent{
				ID:           id,
				Dependencies: dependencies,
				Content: helm.ReleaseDeclaration{
					Name:      name,
					Namespace: namespace,
					Chart:     *chart,
					Values:    values,
					CRDs: helm.CRDs{
						AllowUpgrade: allowUpgrade,
					},
				},
			}

			if len(patches.Unstructureds) != 0 {
				hr.Content.Patches = patches
			}

			instances = append(instances, hr)
		}
	}

	return instances, nil
}

func decodeValues(componentValue cue.Value) (helm.Values, error) {
	valuesValue, err := getValue(componentValue, "values")
	if err != nil {
		return nil, err
	}

	values := map[string]any{}
	if err := valuesValue.Decode(&values); err != nil {
		return nil, err
	}
	return values, nil
}

func decodeChart(componentValue cue.Value) (*helm.Chart, error) {
	chartValue, err := getValue(componentValue, "chart")
	if err != nil {
		return nil, err
	}

	chartName, err := getStringValue(*chartValue, "name")
	if err != nil {
		return nil, err
	}

	repoURL, err := getStringValue(*chartValue, "repoURL")
	if err != nil {
		return nil, err
	}

	version, err := getStringValue(*chartValue, "version")
	if err != nil {
		return nil, err
	}

	authValue, err := getOptionalValue(*chartValue, "auth")
	if err != nil {
		return nil, err
	}

	var optionalAuth *helm.Auth
	if authValue != nil {
		auth := &helm.Auth{}
		if err := authValue.Decode(auth); err != nil {
			return nil, err
		}
		optionalAuth = auth
	}

	chart := &helm.Chart{
		Name:    chartName,
		RepoURL: repoURL,
		Version: version,
		Auth:    optionalAuth,
	}

	return chart, nil
}

func decodeValue(
	value cue.Value,
	content map[string]any,
	metadata MetadataNode,
	evaluateMetadata bool,
) (*AttributeInfo, error) {
	if value.Err() != nil {
		return nil, value.Err()
	}

	switch value.Kind() {
	case cue.StructKind:
		return decodeStruct(value, content, metadata, evaluateMetadata)

	case cue.ListKind:
		return decodeList(value, content, metadata, evaluateMetadata)

	case cue.BottomKind:
		if defaultValue, exists := value.Default(); exists {
			return decodeValue(defaultValue, content, metadata, evaluateMetadata)
		}
		return nil, nil

	default:
		return decodePrimitives(value, content, metadata, evaluateMetadata)
	}

}

func decodeList(
	value cue.Value,
	content map[string]any,
	metadata MetadataNode,
	evaluateMetadata bool,
) (*AttributeInfo, error) {
	if value.Kind() != cue.ListKind {
		return nil, nil
	}

	name := getLabel(value)

	attrInfo := &AttributeInfo{
		HasIgnoreConflictAttributes: false,
	}

	ignoreAttr := getIgnoreAttribute(value)

	if name != "" && evaluateMetadata {
		switch ignoreAttr {

		case kube.OnConflict:
			metadata[name] = &FieldMetadata{
				IgnoreAttr: kube.OnConflict,
			}

			attrInfo.HasIgnoreConflictAttributes = true

		}
	}

	list, err := handleList(value)
	if err != nil {
		return nil, err
	}

	content[name] = list

	return attrInfo, nil
}

func decodePrimitives(
	value cue.Value,
	content map[string]any,
	metadata MetadataNode,
	evaluateMetadata bool,
) (*AttributeInfo, error) {
	name := getLabel(value)

	attrInfo := &AttributeInfo{
		HasIgnoreConflictAttributes: false,
	}

	ignoreAttr := getIgnoreAttribute(value)

	if name != "" && evaluateMetadata {
		switch ignoreAttr {

		case kube.OnConflict:
			metadata[name] = &FieldMetadata{
				IgnoreAttr: kube.OnConflict,
			}

			attrInfo.HasIgnoreConflictAttributes = true

		}
	}

	concreteValue, err := getConcreteValue(value)
	if err != nil {
		return nil, err
	}

	if concreteValue != nil {
		content[name] = concreteValue
	}

	return attrInfo, nil
}

func getConcreteValue(value cue.Value) (any, error) {

	switch value.Kind() {
	case cue.BottomKind:
		if defaultValue, exists := value.Default(); exists {
			concreteValue, err := getConcreteValue(defaultValue)
			if err != nil {
				return nil, err
			}
			return concreteValue, nil
		}

	case cue.StringKind:
		concreteValue, err := value.String()
		if err != nil {
			return nil, err
		}
		return concreteValue, nil

	case cue.BytesKind:
		concreteValue, err := value.Bytes()
		if err != nil {
			return nil, err
		}
		return concreteValue, nil

	case cue.BoolKind:
		concreteValue, err := value.Bool()
		if err != nil {
			return nil, err
		}
		return concreteValue, nil

	case cue.FloatKind:
		concreteValue, err := value.Float64()
		if err != nil {
			return nil, err
		}
		return concreteValue, nil

	case cue.IntKind:
		concreteValue, err := value.Int64()
		if err != nil {
			return nil, err
		}
		return concreteValue, nil

	}

	return nil, nil
}

func decodeStruct(
	value cue.Value,
	content map[string]any,
	metadata MetadataNode,
	evaluateMetadata bool,
) (*AttributeInfo, error) {
	if value.Kind() != cue.StructKind {
		return nil, nil
	}

	name := getLabel(value)

	attrInfo := &AttributeInfo{
		HasIgnoreConflictAttributes: false,
	}

	ignoreAttr := getIgnoreAttribute(value)

	var childContent map[string]any
	var childMetadataNode MetadataNode

	if name != "" {

		if evaluateMetadata {
			switch ignoreAttr {

			case kube.OnConflict:
				metadata[name] = &FieldMetadata{
					IgnoreAttr: kube.OnConflict,
				}

				evaluateMetadata = false

				attrInfo.HasIgnoreConflictAttributes = true

			default:
				childMetadataNode = MetadataNode{}

			}
		}

		childContent = map[string]any{}

	} else {
		childContent = content
		childMetadataNode = metadata
	}

	iter, err := value.Fields()
	if err != nil {
		return nil, err
	}

	for iter.Next() {
		childValue := iter.Value()
		childAttrInfo, err := decodeValue(
			childValue,
			childContent,
			childMetadataNode,
			evaluateMetadata,
		)
		if err != nil {
			return nil, err
		}

		if !attrInfo.HasIgnoreConflictAttributes && childAttrInfo.HasIgnoreConflictAttributes {
			attrInfo.HasIgnoreConflictAttributes = true
		}
	}

	if name != "" {
		if len(childContent) != 0 {
			content[name] = childContent
		}

		if attrInfo.HasIgnoreConflictAttributes && evaluateMetadata {
			metadata[name] = &childMetadataNode
		}
	}

	return attrInfo, nil
}

func handleList(value cue.Value) ([]any, error) {
	iter, err := value.List()
	if err != nil {
		return nil, err
	}

	listContent := []any{}
	for iter.Next() {
		childValue := iter.Value()

		switch childValue.Kind() {

		case cue.StructKind:
			childContent := map[string]any{}
			if _, err := decodeStruct(childValue, childContent, nil, false); err != nil {
				return nil, err
			}
			listContent = append(listContent, childContent)

		case cue.ListKind:
			childList, err := handleList(childValue)
			if err != nil {
				return nil, err
			}
			listContent = append(listContent, childList)

		case cue.StringKind:
			actualValue, err := childValue.String()
			if err != nil {
				return nil, err
			}
			listContent = append(listContent, actualValue)

		case cue.IntKind:
			actualValue, err := childValue.Int64()
			if err != nil {
				return nil, err
			}
			listContent = append(listContent, actualValue)

		case cue.FloatKind:
			actualValue, err := childValue.Float64()
			if err != nil {
				return nil, err
			}
			listContent = append(listContent, actualValue)

		case cue.BoolKind:
			actualValue, err := childValue.Bool()
			if err != nil {
				return nil, err
			}
			listContent = append(listContent, actualValue)

		case cue.BytesKind:
			actualValue, err := childValue.Bytes()
			if err != nil {
				return nil, err
			}
			listContent = append(listContent, actualValue)
		}
	}

	return listContent, nil
}

func getIgnoreAttribute(value cue.Value) kube.ManifestIgnoreAttribute {
	attributes := value.Attributes(cue.ValueAttr)

	for _, attr := range attributes {
		if attr.Name() == ignoreAttr {
			switch attr.Name() {
			case ignoreAttr:
				return kube.OnConflict
			}
		}
	}

	return kube.None
}

func getStringValue(value cue.Value, key string) (string, error) {
	parsedValue := value.LookupPath(cue.ParsePath(key))
	if parsedValue.Err() != nil {
		return "", parsedValue.Err()
	}
	stringValue, err := parsedValue.String()
	if err != nil {
		return "", err
	}
	return stringValue, nil
}

func getBoolValue(value cue.Value, key string) (bool, error) {
	parsedValue := value.LookupPath(cue.ParsePath(key))
	if parsedValue.Err() != nil {
		return false, parsedValue.Err()
	}
	boolValue, err := parsedValue.Bool()
	if err != nil {
		return false, err
	}
	return boolValue, nil
}

func getStringSliceValue(value cue.Value, key string) ([]string, error) {
	parsedValue := value.LookupPath(cue.ParsePath(key))
	if parsedValue.Err() != nil {
		return nil, missingFieldError(key)
	}
	stringSlice := []string{}
	if err := parsedValue.Decode(&stringSlice); err != nil {
		return nil, err
	}
	return stringSlice, nil
}

func getValue(value cue.Value, key string) (*cue.Value, error) {
	parsedValue := value.LookupPath(cue.ParsePath(key))
	if parsedValue.Err() != nil {
		return nil, parsedValue.Err()
	}
	return &parsedValue, nil
}

func getOptionalValue(value cue.Value, key string) (*cue.Value, error) {
	parsedValue := value.LookupPath(cue.ParsePath(key))
	if parsedValue.Err() != nil && parsedValue.Exists() {
		return nil, parsedValue.Err()
	}

	if !parsedValue.Exists() {
		return nil, nil
	}

	return &parsedValue, nil
}

func validateManifest(instance Manifest) error {
	obj := instance.Content.Object

	_, found := obj["apiVersion"]
	if !found {
		return fmt.Errorf(
			"%w [Manifest: %s]",
			missingFieldError("apiVersion"),
			obj,
		)
	}

	_, found = obj["kind"]
	if !found {
		return fmt.Errorf(
			"%w [Manifest: %s]",
			missingFieldError("kind"),
			obj,
		)
	}

	metadata, ok := obj["metadata"].(map[string]any)
	if !ok {
		return fmt.Errorf(
			"%w: %s field not found or wrong format",
			ErrMissingField,
			"metadata",
		)
	}

	_, found = metadata["name"]
	if !found {
		return missingFieldError("metadata.name")
	}

	return nil
}

func missingFieldError(key string) error {
	return fmt.Errorf("%w: %s field not found", ErrMissingField, key)
}

func getLabel(value cue.Value) string {
	selector := value.Path().Selectors()
	len := len(selector)
	if len < 1 {
		return ""
	}

	label := selector[len-1].String()
	if _, err := strconv.Atoi(label); err == nil {
		return ""
	}

	return strings.ReplaceAll(label, "\"", "")
}
