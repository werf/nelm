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

package action

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/werf/3p-helm/pkg/kube"
	"github.com/werf/3p-helm/pkg/releaseutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/resource"
)

func existingResourceConflict(resources kube.ResourceList, releaseName, releaseNamespace string) (kube.ResourceList, error) {
	var requireUpdate kube.ResourceList

	err := resources.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}

		helper := resource.NewHelper(info.Client, info.Mapping)
		existing, err := helper.Get(info.Namespace, info.Name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return errors.Wrapf(err, "could not get information about the resource %s", releaseutil.ResourceString(info))
		}

		// Allow adoption of the resource if it is managed by Helm and is annotated with correct release name and namespace.
		if err := releaseutil.CheckOwnership(existing, releaseName, releaseNamespace); err != nil {
			return fmt.Errorf("%s exists and cannot be imported into the current release: %s", releaseutil.ResourceString(info), err)
		}

		requireUpdate.Append(info)
		return nil
	})

	return requireUpdate, err
}

func ExistingResourceConflict(resources kube.ResourceList, releaseName, releaseNamespace string) (kube.ResourceList, error) {
	return existingResourceConflict(resources, releaseName, releaseNamespace)
}
