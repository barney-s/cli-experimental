/*
Copyright 2019 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resourceconfig

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sigs.k8s.io/kustomize/pkg/inventory"
	"strings"

	"sigs.k8s.io/kustomize/pkg/ifc"
	"sigs.k8s.io/yaml"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
	"sigs.k8s.io/kustomize/pkg/fs"
	"sigs.k8s.io/kustomize/pkg/ifc/transformer"
	"sigs.k8s.io/kustomize/pkg/loader"
	"sigs.k8s.io/kustomize/pkg/plugins"
	"sigs.k8s.io/kustomize/pkg/resmap"
	"sigs.k8s.io/kustomize/pkg/target"
	"sigs.k8s.io/kustomize/pkg/types"
)

// ConfigProvider provides runtime.Objects for a path
type ConfigProvider interface {
	// IsSupported returns true if the ConfigProvider supports the given path
	IsSupported(path string) bool

	// GetConfig returns the Resource Config as runtime.Objects
	GetConfig(path string) ([]*unstructured.Unstructured, error)

	// GetPruneConfig returns the Resource Config used for pruning
	GetPruneConfig(path string) (*unstructured.Unstructured, error)
}

var _ ConfigProvider = &KustomizeProvider{}
var _ ConfigProvider = &RawConfigFileProvider{}
var _ ConfigProvider = &RawConfigHTTPProvider{}

// KustomizeProvider provides configs from Kusotmize targets
type KustomizeProvider struct {
	RF *resmap.Factory
	TF transformer.Factory
	FS fs.FileSystem
	PC *types.PluginConfig
}

func (p *KustomizeProvider) getKustTarget(path string) (ifc.Loader, *target.KustTarget, error) {
	ldr, err := loader.NewLoader(loader.RestrictionRootOnly, path, p.FS)
	if err != nil {
		return ldr, nil, err
	}
	kt, err := target.NewKustTarget(ldr, p.RF, p.TF, plugins.NewLoader(p.PC, p.RF))
	return ldr, kt, err
}

// IsSupported checks if the path is supported by KustomizeProvider
func (p *KustomizeProvider) IsSupported(path string) bool {
	ldr, _, err := p.getKustTarget(path)
	defer ldr.Cleanup()

	if err != nil {
		return false
	}
	return true
}

// GetConfig returns the resource configs
func (p *KustomizeProvider) GetConfig(path string) ([]*unstructured.Unstructured, error) {
	ldr, kt, err := p.getKustTarget(path)
	if err != nil {
		return nil, err
	}
	defer ldr.Cleanup()
	allResources, err := kt.MakeCustomizedResMap()
	if err != nil {
		return nil, err
	}
	var results []*unstructured.Unstructured
	for _, r := range allResources {
		results = append(results, &unstructured.Unstructured{Object: r.Kunstructured.Map()})
	}
	return results, nil
}

// GetPruneConfig returns the resource configs
func (p *KustomizeProvider) GetPruneConfig(path string) (*unstructured.Unstructured, error) {
	ldr, kt, err := p.getKustTarget(path)
	if err != nil {
		return nil, err
	}
	defer ldr.Cleanup()
	allResources, err := kt.MakePruneConfigMap()
	if err != nil {
		return nil, err
	}
	if len(allResources) > 1 {
		return nil, fmt.Errorf("only allow one object as the Prune config")
	}

	for _, r := range allResources {
		return &unstructured.Unstructured{Object: r.Kunstructured.Map()}, nil
	}

	return nil, nil
}

// RawConfigFileProvider provides configs from raw K8s resources
type RawConfigFileProvider struct{}

// IsSupported checks if a path is a raw K8s configuration file
func (p *RawConfigFileProvider) IsSupported(path string) bool {
	// Don't allow running on kustomization.yaml, prevents weird things like globbing
	if filepath.Base(path) == "kustomization.yaml" {
		return false
	}
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
}

// GetConfig returns the resource configs
func (p *RawConfigFileProvider) GetConfig(path string) ([]*unstructured.Unstructured, error) {
	var values clik8s.ResourceConfigs

	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	objs := strings.Split(string(b), "---")
	for _, o := range objs {
		body := map[string]interface{}{}

		if err := yaml.Unmarshal([]byte(o), &body); err != nil {
			return nil, err
		}
		values = append(values, &unstructured.Unstructured{Object: body})
	}

	return values, nil
}

// GetPruneConfig returns the resource configs
func (p *RawConfigFileProvider) GetPruneConfig(path string) (*unstructured.Unstructured, error) {
	return nil, nil
}

// RawConfigHTTPProvider provides configs from HTTP urls
// TODO: implement RawConfigHTTPProvider
type RawConfigHTTPProvider struct{}

// IsSupported returns if the path points to a HTTP url target
func (p *RawConfigHTTPProvider) IsSupported(path string) bool {
	return false
}

// GetConfig returns the resource configs
func (p *RawConfigHTTPProvider) GetConfig(path string) ([]*unstructured.Unstructured, error) {
	return nil, nil
}

// GetPruneConfig returns the resource configs
func (p *RawConfigHTTPProvider) GetPruneConfig(path string) (*unstructured.Unstructured, error) {
	return nil, nil
}

// GetPruneResources finds the resource used for pruning from a slice of resources
// by looking for a special annotation in the resource
// inventory.InventoryAnnotation
func GetPruneResources(resources []*unstructured.Unstructured) (*unstructured.Unstructured, error) {
	count := 0
	var result *unstructured.Unstructured

	for _, res := range resources {
		annotations := res.GetAnnotations()
		if _, ok := annotations[inventory.InventoryAnnotation]; ok {
			count++
			result = res
		}
	}

	if count == 0 {
		return nil, nil
	}
	if count > 1 {
		return nil, fmt.Errorf("found multiple resources with inventory annotations")
	}
	return result, nil
}
