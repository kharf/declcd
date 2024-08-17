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
	"strings"

	cueErrors "cuelang.org/go/cue/errors"

	"cuelang.org/go/cue"
	internalCue "github.com/kharf/declcd/internal/cue"
	"github.com/kharf/declcd/pkg/helm"
	"github.com/kharf/declcd/pkg/kube"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Manifest = kube.Manifest
type ExtendedUnstructured = kube.ExtendedUnstructured
type FieldMetadata = kube.ManifestFieldMetadata

var (
	ErrMissingField    = errors.New("Missing content field")
	ErrEmptyFieldLabel = errors.New("Unexpected empty field label")
	ErrCUEBuildError   = errors.New("CUE Build Error")
)

const (
	// ignoreAttr is a CUE build attribute a user can define on a field or declaration
	// to tell Declcd to ignore fields or structs when applying Kubernetes Manifests.
	ignoreAttr = "ignore"
	// updateAttr is a CUE build attribute a user can define on a field
	// to tell Declcd to automatically update container images.
	updateAttr = "update"
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

type BuildResult struct {
	Instances          []Instance
	UpdateInstructions []kube.UpdateInstruction
}

// Build accepts options defining which cue package to compile
// and compiles it to a slice of component Instances.
func (b Builder) Build(opts ...buildOptions) (*BuildResult, error) {
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
		return nil, buildError(err)
	}

	iter, err := value.Fields()
	if err != nil {
		return nil, buildError(err)
	}

	var instances []Instance
	var updateInstructions []kube.UpdateInstruction

	for iter.Next() {
		componentValue := iter.Value()

		instanceType, err := getStringValue(componentValue, "type")
		if err != nil {
			return nil, buildError(err)
		}

		id, err := getStringValue(componentValue, "id")
		if err != nil {
			return nil, buildError(err)
		}

		dependencies, err := getStringSliceValue(componentValue, "dependencies")
		if err != nil {
			return nil, buildError(err)
		}

		switch instanceType {
		case "Manifest":
			contentValue, err := getValue(componentValue, "content")
			if err != nil {
				return nil, buildError(err)
			}

			content, metadata, updateInstr, err := decodeValue(*contentValue, nil)
			if err != nil {
				return nil, buildError(err)
			}

			contentNode, ok := content.(map[string]any)
			if !ok {
				return nil, fmt.Errorf(
					"%w: expected content to be of type struct",
					ErrCUEBuildError,
				)
			}

			manifest := Manifest{
				ID:           id,
				Dependencies: dependencies,
				Content: ExtendedUnstructured{
					Unstructured: &unstructured.Unstructured{
						Object: contentNode,
					},
					Metadata: metadata,
				},
			}

			if err := validateManifest(manifest); err != nil {
				return nil, fmt.Errorf("%w: %w", ErrCUEBuildError, err)
			}
			instances = append(instances, &manifest)
			updateInstructions = append(updateInstructions, updateInstr...)

		case "HelmRelease":
			name, err := getStringValue(componentValue, "name")
			if err != nil {
				return nil, buildError(err)
			}

			namespace, err := getStringValue(componentValue, "namespace")
			if err != nil {
				return nil, buildError(err)
			}

			chart, err := decodeChart(componentValue)
			if err != nil {
				return nil, buildError(err)
			}

			values, err := decodeValues(componentValue)
			if err != nil {
				return nil, buildError(err)
			}

			patchesValue, err := getValue(componentValue, "patches")
			if err != nil {
				return nil, buildError(err)
			}

			patchesValueIter, err := patchesValue.List()
			if err != nil {
				return nil, buildError(err)
			}

			patches := helm.NewPatches()
			for patchesValueIter.Next() {
				value := patchesValueIter.Value()
				content, metadata, updateInstr, err := decodeValue(value, nil)
				if err != nil {
					return nil, buildError(err)
				}

				contentNode, ok := content.(map[string]any)
				if !ok {
					return nil, fmt.Errorf(
						"%w: expected patches content to be of type struct",
						ErrCUEBuildError,
					)
				}

				unstr := kube.ExtendedUnstructured{
					Unstructured: &unstructured.Unstructured{
						Object: contentNode,
					},
					Metadata: metadata,
				}

				patches.Put(unstr)
				updateInstructions = append(updateInstructions, updateInstr...)
			}

			crdsValue, err := getValue(componentValue, "crds")
			if err != nil {
				return nil, buildError(err)
			}

			allowUpgrade, err := getBoolValue(*crdsValue, "allowUpgrade")
			if err != nil {
				return nil, buildError(err)
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

	return &BuildResult{
		Instances:          instances,
		UpdateInstructions: updateInstructions,
	}, nil
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
	parentNode map[string]any,
) (any, *kube.ManifestMetadata, []kube.UpdateInstruction, error) {
	if value.Err() != nil {
		return nil, nil, nil, value.Err()
	}

	var err error
	var content any
	var metadata *kube.ManifestMetadata
	var fieldMeta *kube.ManifestFieldMetadata
	var updateInstructions []kube.UpdateInstruction

	switch value.Kind() {
	case cue.StructKind:
		fieldMeta, err = decodeBuildAttributes(value)
		if err != nil {
			return nil, nil, nil, err
		}
		content, metadata, updateInstructions, err = decodeStruct(value)

	case cue.ListKind:
		fieldMeta, err = decodeBuildAttributes(value)
		if err != nil {
			return nil, nil, nil, err
		}
		content, metadata, updateInstructions, err = decodeList(value)

	case cue.BottomKind:
		defaultValue, exists := value.Default()
		if !exists {
			return nil, nil, nil, fmt.Errorf(
				"%w: invalid value %v",
				ErrCUEBuildError,
				getLabel(value),
			)
		}

		return decodeValue(defaultValue, parentNode)

	default:
		var updateInstr *kube.UpdateInstruction
		content, fieldMeta, updateInstr, err = decodeField(value, parentNode)
		if updateInstr != nil {
			updateInstructions = append(updateInstructions, *updateInstr)
		}
	}

	if err != nil {
		return nil, nil, nil, err
	}

	if metadata == nil && fieldMeta != nil {
		metadata = &kube.ManifestMetadata{
			Field: fieldMeta,
		}
	}

	if metadata != nil {
		metadata.Field = fieldMeta
	}

	return content, metadata, updateInstructions, nil
}

func decodeList(
	value cue.Value,
) ([]any, *kube.ManifestMetadata, []kube.UpdateInstruction, error) {
	iter, err := value.List()
	if err != nil {
		return nil, nil, nil, err
	}

	var content []any
	var metadata []kube.ManifestMetadata
	var updateInstructions []kube.UpdateInstruction

	for iter.Next() {
		childValue := iter.Value()
		childContent, childMetadata, childUpdateInstructions, err := decodeValue(childValue, nil)
		if err != nil {
			return nil, nil, nil, err
		}

		content = append(content, childContent)
		if childMetadata != nil {
			metadata = append(metadata, *childMetadata)
		}
		updateInstructions = append(updateInstructions, childUpdateInstructions...)
	}

	if len(metadata) != 0 {
		return content, &kube.ManifestMetadata{
			List: metadata,
		}, updateInstructions, nil
	}

	return content, nil, updateInstructions, nil
}

func decodeField(
	value cue.Value,
	parentNode map[string]any,
) (any, *kube.ManifestFieldMetadata, *kube.UpdateInstruction, error) {
	switch value.Kind() {
	case cue.StringKind:
		fieldMeta, updateInstr, err := decodeStringBuildAttributes(value)
		if err != nil {
			return nil, nil, nil, err
		}
		concreteValue, err := value.String()
		if err != nil {
			return nil, nil, nil, err
		}

		if updateInstr != nil {
			updateInstr.Image = concreteValue
			updateInstr.UnstructuredKey = getLabel(value)
			updateInstr.UnstructuredNode = parentNode
		}

		return concreteValue, fieldMeta, updateInstr, nil

	case cue.BytesKind:
		fieldMeta, err := decodeBuildAttributes(value)
		if err != nil {
			return nil, nil, nil, err
		}
		concreteValue, err := value.Bytes()
		if err != nil {
			return nil, nil, nil, err
		}
		return concreteValue, fieldMeta, nil, nil

	case cue.BoolKind:
		fieldMeta, err := decodeBuildAttributes(value)
		if err != nil {
			return nil, nil, nil, err
		}
		concreteValue, err := value.Bool()
		if err != nil {
			return nil, nil, nil, err
		}
		return concreteValue, fieldMeta, nil, nil

	case cue.FloatKind:
		fieldMeta, err := decodeBuildAttributes(value)
		if err != nil {
			return nil, nil, nil, err
		}
		concreteValue, err := value.Float64()
		if err != nil {
			return nil, nil, nil, err
		}
		return concreteValue, fieldMeta, nil, nil

	case cue.IntKind:
		fieldMeta, err := decodeBuildAttributes(value)
		if err != nil {
			return nil, nil, nil, err
		}
		concreteValue, err := value.Int64()
		if err != nil {
			return nil, nil, nil, err
		}
		return concreteValue, fieldMeta, nil, nil
	}

	return nil, nil, nil, nil
}

func decodeStruct(
	value cue.Value,
) (map[string]any, *kube.ManifestMetadata, []kube.UpdateInstruction, error) {
	iter, err := value.Fields()
	if err != nil {
		return nil, nil, nil, err
	}

	content := map[string]any{}
	nodeMetadata := map[string]kube.ManifestMetadata{}
	var updateInstructions []kube.UpdateInstruction

	for iter.Next() {
		childValue := iter.Value()
		childLabel := getLabel(childValue)
		if childLabel == "" {
			return nil, nil, nil, ErrEmptyFieldLabel
		}

		childContent, childMetadata, childUpdateInstr, err := decodeValue(childValue, content)
		if err != nil {
			return nil, nil, nil, err
		}

		content[childLabel] = childContent
		if childMetadata != nil {
			nodeMetadata[childLabel] = *childMetadata
		}
		updateInstructions = append(updateInstructions, childUpdateInstr...)
	}

	if len(nodeMetadata) != 0 {
		return content, &kube.ManifestMetadata{
			Node: nodeMetadata,
		}, updateInstructions, nil
	}

	return content, nil, updateInstructions, nil
}

func decodeBuildAttributes(value cue.Value) (*FieldMetadata, error) {
	attributes := value.Attributes(cue.ValueAttr)

	var meta *FieldMetadata
	for _, attr := range attributes {
		switch attr.Name() {
		case ignoreAttr:
			if meta == nil {
				meta = new(FieldMetadata)
			}
			meta.IgnoreInstr = kube.OnConflict
		}
	}

	return meta, nil
}

func decodeStringBuildAttributes(
	value cue.Value,
) (*FieldMetadata, *kube.UpdateInstruction, error) {
	attributes := value.Attributes(cue.ValueAttr)

	var meta *FieldMetadata
	var updateInstr *kube.UpdateInstruction
	for _, attr := range attributes {
		switch attr.Name() {
		case ignoreAttr:
			if meta == nil {
				meta = new(FieldMetadata)
			}
			meta.IgnoreInstr = kube.OnConflict

		case updateAttr:
			if meta == nil {
				meta = new(FieldMetadata)
			}
			stratDef, _, err := attr.Lookup(0, "strategy")
			if err != nil {
				return nil, nil, err
			}

			constraint, _, err := attr.Lookup(0, "constraint")
			if err != nil {
				return nil, nil, err
			}

			secretRef, _, err := attr.Lookup(0, "secret")
			if err != nil {
				return nil, nil, err
			}

			strat := kube.Semver
			switch stratDef {
			case "semver":
				strat = kube.Semver
			}

			updateInstr = &kube.UpdateInstruction{
				Strategy:   strat,
				Constraint: constraint,
				SecretRef:  secretRef,
				File:       value.Pos().Filename(),
				Line:       value.Pos().Line(),
			}
		}
	}

	return meta, updateInstr, nil
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
	return strings.ReplaceAll(label, "\"", "")
}

func buildError(err error) error {
	return fmt.Errorf("%w: %s", ErrCUEBuildError, cueErrors.Details(err, nil))
}
