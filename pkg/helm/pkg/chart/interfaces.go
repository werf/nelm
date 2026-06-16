/*
Copyright The Helm Authors.
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

package chart

import (
	common "github.com/werf/nelm/pkg/helm/pkg/chart/common"
)

type Charter any

type Dependency any

type Accessor interface {
	Name() string
	IsRoot() bool
	MetadataAsMap() map[string]any
	Files() []*common.File
	RawFiles() []*common.File
	Templates() []*common.File
	ChartFullPath() string
	IsLibraryChart() bool
	Dependencies() []Charter
	MetaDependencies() []Dependency
	Values() map[string]any
	Schema() []byte
	Deprecated() bool
	RuntimeFiles() []*common.File
	AddRuntimeFile(name string, data []byte)
	RemoveRuntimeFile(name string)
	Charter() Charter
}

type DependencyAccessor interface {
	Name() string
	Alias() string
}
