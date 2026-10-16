package plan

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/kube"
	"github.com/werf/nelm/v2/pkg/kube/fake"
	"github.com/werf/nelm/v2/pkg/resource"
	"github.com/werf/nelm/v2/pkg/resource/spec"
)

func TestBuildResourceInfosDeletionErrorLifecycle(t *testing.T) {
	cf, err := fake.NewClientFactory(context.Background())
	require.NoError(t, err)

	delErr := errors.New("deletion analysis failed")
	started := make(chan struct{})
	failed := make(chan struct{})
	release := make(chan struct{})
	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	calls := make(chan context.Context, 1)
	client := &analysisLifecycleKubeClient{KubeClienter: cf.KubeClient()}
	client.get = func(_ context.Context, meta *spec.ResourceMeta, _ kube.KubeClientGetOptions) (*unstructured.Unstructured, error) {
		if meta.Name == "delete" {
			<-started
			close(failed)

			return nil, delErr
		}

		close(started)
		<-release

		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, meta.Name)
	}

	result := make(chan error, 1)
	go func() {
		_, _, err := BuildResourceInfos(context.Background(), common.DeployTypeUpgrade, "release", "default",
			[]*resource.InstallableResource{lifecycleInstallable("install", common.StageInstall)},
			[]*resource.DeletableResource{{ResourceMeta: makeResourceMeta("delete", "default", gvkConfigMap)}}, false,
			&analysisLifecycleClientFactory{ClientFactorier: cf, client: client}, BuildResourceInfosOptions{
				OnResourceAnalyzed: func(ctx context.Context) {
					close(callbackEntered)
					<-releaseCallback

					calls <- ctx
				},
			})
		result <- err
	}()

	select {
	case <-failed:
	case <-time.After(10 * time.Second):
		t.Fatal("deletion analysis did not fail")
	}

	select {
	case <-result:
		t.Fatal("returned before installation worker finished")
	default:
	}

	close(release)

	select {
	case <-callbackEntered:
	case <-time.After(10 * time.Second):
		t.Fatal("installation callback did not start")
	}

	select {
	case <-result:
		t.Fatal("returned before installation callback finished")
	default:
	}

	close(releaseCallback)

	select {
	case err = <-result:
	case <-time.After(10 * time.Second):
		t.Fatal("BuildResourceInfos did not finish")
	}

	close(calls)
	require.ErrorIs(t, err, delErr)
	require.ErrorContains(t, err, "wait for prev release resource pool: build deletable resource info:")
	assert.Len(t, calls, 1)
}

func TestBuildResourceInfosInstallErrorLifecycle(t *testing.T) {
	for _, outcome := range []string{"canceled", "success after cancellation", "deletion error"} {
		t.Run(outcome, func(t *testing.T) {
			cf, err := fake.NewClientFactory(context.Background())
			require.NoError(t, err)

			instErr := errors.New("install analysis failed")
			delErr := errors.New("delete analysis failed")
			started := make(chan struct{})
			canceled := make(chan struct{})
			release := make(chan struct{})
			workerFinished := make(chan struct{})
			callbackEntered := make(chan struct{})
			releaseCallback := make(chan struct{})
			callbackFinished := make(chan struct{})
			calls := make(chan context.Context, 1)

			type contextKey struct{}

			key := contextKey{}
			ctx := context.WithValue(context.Background(), key, "lifecycle")
			client := &analysisLifecycleKubeClient{KubeClienter: cf.KubeClient()}
			client.get = func(ctx context.Context, meta *spec.ResourceMeta, _ kube.KubeClientGetOptions) (*unstructured.Unstructured, error) {
				if meta.Name == "install" {
					<-started

					return nil, instErr
				}

				defer close(workerFinished)

				close(started)

				select {
				case <-ctx.Done():
					close(canceled)
				case <-release:
				}

				<-release

				switch outcome {
				case "canceled":
					return nil, context.Canceled
				case "deletion error":
					return nil, delErr
				default:
					return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, meta.Name)
				}
			}
			factory := &analysisLifecycleClientFactory{ClientFactorier: cf, client: client}

			result := make(chan error, 1)
			go func() {
				_, _, err := BuildResourceInfos(ctx, common.DeployTypeUpgrade, "release", "default",
					[]*resource.InstallableResource{lifecycleInstallable("install", common.StageInstall)},
					[]*resource.DeletableResource{{ResourceMeta: makeResourceMeta("delete", "default", gvkConfigMap)}}, false, factory, BuildResourceInfosOptions{
						NetworkParallelism: 2,
						OnResourceAnalyzed: func(ctx context.Context) {
							close(callbackEntered)
							<-releaseCallback

							calls <- ctx

							close(callbackFinished)
						},
					})
				result <- err
			}()

			var returnedErr error

			earlyReturn := false
			select {
			case <-canceled:
				select {
				case returnedErr = <-result:
					earlyReturn = true

					t.Error("BuildResourceInfos returned before the deletion worker finished")
				default:
				}
			case returnedErr = <-result:
				earlyReturn = true

				t.Error("BuildResourceInfos returned before canceling and joining the deletion worker")
			case <-time.After(10 * time.Second):
				t.Error("installation failure did not cancel the deletion worker")
			}

			close(release)

			if outcome == "success after cancellation" {
				select {
				case <-callbackEntered:
				case <-time.After(10 * time.Second):
					t.Error("successful deletion analysis did not invoke callback")
				}

				if !earlyReturn {
					select {
					case returnedErr = <-result:
						earlyReturn = true

						t.Error("BuildResourceInfos returned while callback was running")
					default:
					}
				}
			}

			close(releaseCallback)

			if !earlyReturn {
				select {
				case returnedErr = <-result:
				case <-time.After(10 * time.Second):
					t.Fatal("BuildResourceInfos did not finish after workers were released")
				}
			}

			if !earlyReturn {
				close(calls)

				select {
				case <-workerFinished:
				default:
					t.Error("returned before deletion Get finished")
				}

				if outcome == "success after cancellation" {
					select {
					case <-callbackFinished:
					default:
						t.Error("returned before deletion callback finished")
					}
				}
			}

			select {
			case <-workerFinished:
			case <-time.After(10 * time.Second):
				t.Fatal("deletion worker did not finish")
			}

			if outcome == "success after cancellation" {
				select {
				case <-callbackFinished:
				case <-time.After(10 * time.Second):
					t.Fatal("callback did not finish")
				}

				assert.Len(t, calls, 1)
				assert.Equal(t, "lifecycle", (<-calls).Value(key))
			} else {
				assert.Empty(t, calls)
			}

			if earlyReturn {
				close(calls)
			}

			require.ErrorIs(t, returnedErr, instErr)
			require.ErrorContains(t, returnedErr, "wait for resource pool: build installable resource info:")
		})
	}
}

func TestBuildResourceInfosSuccessfulMixedLifecycle(t *testing.T) {
	for _, withCallback := range []bool{true, false} {
		name := "callback"
		if !withCallback {
			name = "nil callback"
		}

		t.Run(name, func(t *testing.T) {
			cf, err := fake.NewClientFactory(context.Background())
			require.NoError(t, err)

			policySkip := lifecycleInstallable("policy", common.StageInstall)
			policySkip.ResourcePolicies = []common.ResourcePolicy{common.ResourcePolicySkipCreate}
			instResources := []*resource.InstallableResource{
				lifecycleInstallable("multi-stage", common.StagePreInstall, common.StagePostInstall),
				lifecycleInstallable("stage-skip"), policySkip, lifecycleInstallable("normal", common.StageInstall),
			}
			delResources := []*resource.DeletableResource{
				{ResourceMeta: makeResourceMeta("absent", "default", gvkConfigMap)},
				{ResourceMeta: makeResourceMeta("policy-skip", "default", gvkConfigMap), ResourcePolicies: []common.ResourcePolicy{common.ResourcePolicySkipDelete}},
			}

			type contextKey struct{}

			key := contextKey{}
			ctx := context.WithValue(context.Background(), key, "mixed")
			calls := make(chan context.Context, 6)
			releaseCallbacks := make(chan struct{})

			opts := BuildResourceInfosOptions{NetworkParallelism: 10}
			if withCallback {
				opts.OnResourceAnalyzed = func(ctx context.Context) {
					calls <- ctx

					<-releaseCallbacks
				}
			}

			finished := make(chan struct{})

			var (
				instInfos []*InstallableResourceInfo
				delInfos  []*DeletableResourceInfo
			)

			go func() {
				instInfos, delInfos, err = BuildResourceInfos(ctx, common.DeployTypeUpgrade, "release", "default", instResources, delResources, false, cf, opts)

				close(finished)
			}()

			if withCallback {
				select {
				case workerCtx := <-calls:
					assert.Equal(t, "mixed", workerCtx.Value(key))
				case <-time.After(10 * time.Second):
					t.Fatal("callback did not start")
				}

				select {
				case <-finished:
					t.Fatal("returned while callbacks were blocked")
				default:
				}
			}

			close(releaseCallbacks)

			select {
			case <-finished:
			case <-time.After(10 * time.Second):
				t.Fatal("BuildResourceInfos did not finish")
			}

			close(calls)
			require.NoError(t, err)
			require.Len(t, instInfos, 4)
			require.Len(t, delInfos, 2)

			if withCallback {
				assert.Len(t, calls, 5)

				for workerCtx := range calls {
					assert.Equal(t, "mixed", workerCtx.Value(key))
				}
			} else {
				assert.Empty(t, calls)
			}
		})
	}
}
