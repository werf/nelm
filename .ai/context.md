# AI context bundle

This is rebuilt on every commit to main, so might not reflect uncommited changes or changes in branches.

## Repository file tree

.
├── cmd
│   └── nelm
│       ├── chart.go
│       ├── chart_dependency.go
│       ├── chart_dependency_download.go
│       ├── chart_dependency_update.go
│       ├── chart_download.go
│       ├── chart_lint.go
│       ├── chart_pack.go
│       ├── chart_render.go
│       ├── chart_secret.go
│       ├── chart_secret_file.go
│       ├── chart_secret_file_decrypt.go
│       ├── chart_secret_file_edit.go
│       ├── chart_secret_file_encrypt.go
│       ├── chart_secret_key.go
│       ├── chart_secret_key_create.go
│       ├── chart_secret_key_rotate.go
│       ├── chart_secret_values_file.go
│       ├── chart_secret_values_file_decrypt.go
│       ├── chart_secret_values_file_edit.go
│       ├── chart_secret_values_file_encrypt.go
│       ├── chart_upload.go
│       ├── common.go
│       ├── common_flags.go
│       ├── groups.go
│       ├── main.go
│       ├── release.go
│       ├── release_get.go
│       ├── release_history.go
│       ├── release_install.go
│       ├── release_list.go
│       ├── release_list_legacy.go
│       ├── release_plan.go
│       ├── release_plan_install.go
│       ├── release_rollback.go
│       ├── release_uninstall.go
│       ├── release_uninstall_legacy.go
│       ├── repo.go
│       ├── repo_add.go
│       ├── repo_login.go
│       ├── repo_logout.go
│       ├── repo_remove.go
│       ├── repo_update.go
│       ├── root.go
│       ├── usage.go
│       └── version.go
├── internal
│   ├── chart
│   │   ├── chart_download.go
│   │   └── chart_render.go
│   ├── kube
│   │   ├── fake
│   │   │   ├── client_discovery.go
│   │   │   ├── client_dynamic.go
│   │   │   ├── client_static.go
│   │   │   └── factory.go
│   │   ├── client_discovery.go
│   │   ├── client_dynamic.go
│   │   ├── client_kube.go
│   │   ├── client_mapper.go
│   │   ├── client_static.go
│   │   ├── common.go
│   │   ├── config.go
│   │   ├── error.go
│   │   ├── factory.go
│   │   └── legacy_client_getter.go
│   ├── legacy
│   │   └── deploy
│   │       ├── resources_waiter.go
│   │       └── stages_splitter.go
│   ├── lock
│   │   └── lock_manager.go
│   ├── plan
│   │   ├── export_test.go
│   │   ├── operation.go
│   │   ├── operation_config.go
│   │   ├── plan.go
│   │   ├── plan_build.go
│   │   ├── plan_build_test.go
│   │   ├── plan_execute.go
│   │   ├── planned_changes.go
│   │   ├── release_info.go
│   │   ├── resource_info.go
│   │   ├── resource_info_test.go
│   │   ├── sort.go
│   │   └── validate.go
│   ├── release
│   │   ├── history.go
│   │   ├── release.go
│   │   └── release_storage.go
│   ├── resource
│   │   ├── spec
│   │   │   ├── patch.go
│   │   │   ├── resource_match.go
│   │   │   ├── resource_meta.go
│   │   │   ├── resource_spec.go
│   │   │   ├── sort.go
│   │   │   ├── transform.go
│   │   │   ├── unstruct.go
│   │   │   ├── unstruct_test.go
│   │   │   └── util.go
│   │   ├── dependency.go
│   │   ├── metadata.go
│   │   ├── resource.go
│   │   ├── resource_test.go
│   │   ├── sensitive.go
│   │   ├── sensitive_test.go
│   │   ├── sort.go
│   │   └── validate.go
│   ├── test
│   │   └── comparer.go
│   ├── track
│   │   └── progress_tables.go
│   └── util
│       ├── diff.go
│       ├── int.go
│       ├── json.go
│       ├── multierror.go
│       ├── properties.go
│       └── string.go
├── pkg
│   ├── action
│   │   ├── action_suite_test.go
│   │   ├── chart_lint.go
│   │   ├── chart_render.go
│   │   ├── common.go
│   │   ├── error.go
│   │   ├── release_get.go
│   │   ├── release_install.go
│   │   ├── release_list.go
│   │   ├── release_plan_install.go
│   │   ├── release_rollback.go
│   │   ├── release_uninstall.go
│   │   ├── release_uninstall_legacy.go
│   │   ├── secret_file_decrypt.go
│   │   ├── secret_file_edit.go
│   │   ├── secret_file_encrypt.go
│   │   ├── secret_key_create.go
│   │   ├── secret_key_rotate.go
│   │   ├── secret_values_file_decrypt.go
│   │   ├── secret_values_file_edit.go
│   │   ├── secret_values_file_encrypt.go
│   │   └── version.go
│   ├── common
│   │   ├── common.go
│   │   └── options.go
│   ├── featgate
│   │   └── feat.go
│   ├── legacy
│   │   └── secret
│   │       ├── common.go
│   │       ├── decrypt.go
│   │       ├── edit.go
│   │       ├── encrypt.go
│   │       └── rotate.go
│   └── log
│       ├── init.go
│       ├── logger.go
│       └── logger_logboek.go
├── resources
│   └── images
│       ├── global-delivery-architecture.png
│       ├── graph.png
│       ├── graph.svg
│       ├── nelm-release-install.gif
│       ├── nelm-release-install.png
│       └── nelm-release-plan-install.png
├── scripts
│   ├── builder
│   │   └── Dockerfile
│   └── verify-dist-binaries.sh
├── AGENTS.md
├── ARCHITECTURE.md
├── CHANGELOG.md
├── CLAUDE.md -> AGENTS.md
├── CONTRIBUTING.md
├── GEMINI.md -> AGENTS.md
├── LICENSE
├── OWNERS
├── README.md
├── Taskfile.dist.yaml
├── go.mod
├── go.sum
├── nelm.asc
├── trdl.yaml
└── trdl_channels.yaml


## Imports

Imports for each package in the repository. Output of go list command.

github.com/werf/nelm/cmd/nelm -> bytes cmp context fmt github.com/chanced/caps github.com/pkg/errors github.com/samber/lo github.com/spf13/cobra github.com/spf13/pflag github.com/werf/3p-helm/cmd/helm github.com/werf/3p-helm/pkg/chart/loader github.com/werf/common-go/pkg/cli github.com/werf/logboek github.com/werf/logboek/pkg/types github.com/werf/nelm/pkg/action github.com/werf/nelm/pkg/common github.com/werf/nelm/pkg/featgate github.com/werf/nelm/pkg/log os runtime slices sort strconv strings text/template time unicode
github.com/werf/nelm/internal/chart -> bytes context fmt github.com/samber/lo github.com/werf/3p-helm/pkg/action github.com/werf/3p-helm/pkg/chart github.com/werf/3p-helm/pkg/chart/loader github.com/werf/3p-helm/pkg/chartutil github.com/werf/3p-helm/pkg/cli github.com/werf/3p-helm/pkg/cli/values github.com/werf/3p-helm/pkg/downloader github.com/werf/3p-helm/pkg/engine github.com/werf/3p-helm/pkg/getter github.com/werf/3p-helm/pkg/helmpath github.com/werf/3p-helm/pkg/registry github.com/werf/3p-helm/pkg/releaseutil github.com/werf/3p-helm/pkg/repo github.com/werf/3p-helm/pkg/strvals github.com/werf/3p-helm/pkg/werf/helmopts github.com/werf/nelm/internal/kube github.com/werf/nelm/internal/resource/spec github.com/werf/nelm/pkg/common github.com/werf/nelm/pkg/featgate github.com/werf/nelm/pkg/log io k8s.io/client-go/discovery net/url os path path/filepath sigs.k8s.io/yaml sort strings unicode
github.com/werf/nelm/internal/kube -> context encoding/base64 fmt github.com/jellydator/ttlcache/v3 github.com/samber/lo github.com/werf/nelm/internal/resource/spec github.com/werf/nelm/pkg/common github.com/werf/nelm/pkg/log k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1 k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1 k8s.io/apimachinery/pkg/api/errors k8s.io/apimachinery/pkg/api/meta k8s.io/apimachinery/pkg/api/validation k8s.io/apimachinery/pkg/apis/meta/v1 k8s.io/apimachinery/pkg/apis/meta/v1/unstructured k8s.io/apimachinery/pkg/runtime/schema k8s.io/apimachinery/pkg/types k8s.io/cli-runtime/pkg/genericclioptions k8s.io/client-go/discovery k8s.io/client-go/discovery/cached/disk k8s.io/client-go/dynamic k8s.io/client-go/kubernetes k8s.io/client-go/kubernetes/scheme k8s.io/client-go/rest k8s.io/client-go/restmapper k8s.io/client-go/tools/clientcmd k8s.io/client-go/tools/clientcmd/api k8s.io/client-go/util/homedir os path/filepath reflect regexp strings sync time
github.com/werf/nelm/internal/kube/fake -> context fmt github.com/chanced/caps github.com/evanphx/json-patch github.com/samber/lo github.com/werf/nelm/internal/kube github.com/werf/nelm/internal/resource/spec k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1 k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1 k8s.io/apimachinery/pkg/api/meta k8s.io/apimachinery/pkg/apis/meta/v1 k8s.io/apimachinery/pkg/apis/meta/v1/unstructured k8s.io/apimachinery/pkg/runtime k8s.io/apimachinery/pkg/types k8s.io/apimachinery/pkg/util/json k8s.io/client-go/discovery k8s.io/client-go/discovery/fake k8s.io/client-go/dynamic k8s.io/client-go/dynamic/fake k8s.io/client-go/kubernetes k8s.io/client-go/kubernetes/fake k8s.io/client-go/kubernetes/scheme k8s.io/client-go/testing reflect
github.com/werf/nelm/internal/legacy/deploy -> context fmt github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1 github.com/fluxcd/flagger/pkg/client/clientset/versioned/scheme github.com/werf/3p-helm/pkg/kube github.com/werf/3p-helm/pkg/phases/stages github.com/werf/kubedog/pkg/kube github.com/werf/kubedog/pkg/tracker github.com/werf/kubedog/pkg/tracker/resid github.com/werf/kubedog/pkg/trackers/elimination github.com/werf/kubedog/pkg/trackers/rollout/multitrack github.com/werf/kubedog/pkg/trackers/rollout/multitrack/generic github.com/werf/logboek k8s.io/api/apps/v1 k8s.io/api/apps/v1beta1 k8s.io/api/apps/v1beta2 k8s.io/api/batch/v1 k8s.io/api/extensions/v1beta1 k8s.io/apimachinery/pkg/api/meta k8s.io/apimachinery/pkg/apis/meta/v1 k8s.io/apimachinery/pkg/apis/meta/v1/unstructured k8s.io/apimachinery/pkg/runtime k8s.io/apimachinery/pkg/runtime/schema k8s.io/cli-runtime/pkg/resource k8s.io/client-go/kubernetes/scheme math os regexp sort strconv strings time
github.com/werf/nelm/internal/lock -> context fmt github.com/werf/common-go/pkg/locker_with_retry github.com/werf/lockgate github.com/werf/lockgate/pkg/distributed_locker github.com/werf/nelm/internal/kube github.com/werf/nelm/internal/resource/spec github.com/werf/nelm/pkg/log k8s.io/apimachinery/pkg/apis/meta/v1/unstructured k8s.io/apimachinery/pkg/runtime/schema
github.com/werf/nelm/internal/plan -> bytes context fmt github.com/dominikbraun/graph github.com/dominikbraun/graph/draw github.com/goccy/go-yaml github.com/gookit/color github.com/mitchellh/copystructure github.com/pkg/errors github.com/samber/lo github.com/sourcegraph/conc/pool github.com/wI2L/jsondiff github.com/werf/3p-helm/pkg/release github.com/werf/kubedog/pkg/informer github.com/werf/kubedog/pkg/trackers/dyntracker github.com/werf/kubedog/pkg/trackers/dyntracker/logstore github.com/werf/kubedog/pkg/trackers/dyntracker/statestore github.com/werf/kubedog/pkg/trackers/dyntracker/util github.com/werf/kubedog/pkg/trackers/rollout/multitrack github.com/werf/nelm/internal/kube github.com/werf/nelm/internal/release github.com/werf/nelm/internal/resource github.com/werf/nelm/internal/resource/spec github.com/werf/nelm/internal/util github.com/werf/nelm/pkg/common github.com/werf/nelm/pkg/log k8s.io/apimachinery/pkg/apis/meta/v1 k8s.io/apimachinery/pkg/apis/meta/v1/unstructured k8s.io/apimachinery/pkg/types k8s.io/apimachinery/pkg/util/json regexp sort strings time
github.com/werf/nelm/internal/release -> context fmt github.com/google/go-cmp/cmp github.com/google/go-cmp/cmp/cmpopts github.com/samber/lo github.com/werf/3p-helm/pkg/action github.com/werf/3p-helm/pkg/chart github.com/werf/3p-helm/pkg/chartutil github.com/werf/3p-helm/pkg/release github.com/werf/3p-helm/pkg/releaseutil github.com/werf/3p-helm/pkg/storage github.com/werf/3p-helm/pkg/storage/driver github.com/werf/3p-helm/pkg/time github.com/werf/nelm/internal/kube github.com/werf/nelm/internal/resource/spec github.com/werf/nelm/pkg/common github.com/werf/nelm/pkg/log hash hash/fnv k8s.io/apimachinery/pkg/apis/meta/v1/unstructured k8s.io/client-go/kubernetes k8s.io/client-go/kubernetes/scheme sigs.k8s.io/yaml slices sort strings sync unicode
github.com/werf/nelm/internal/resource -> context crypto/sha256 encoding/json fmt github.com/ohler55/ojg/jp github.com/samber/lo github.com/werf/3p-helm/pkg/release github.com/werf/kubedog/pkg/trackers/rollout/multitrack github.com/werf/nelm/internal/kube github.com/werf/nelm/internal/resource/spec github.com/werf/nelm/internal/util github.com/werf/nelm/pkg/common github.com/werf/nelm/pkg/featgate k8s.io/api/core/v1 k8s.io/apimachinery/pkg/api/meta k8s.io/apimachinery/pkg/apis/meta/v1 k8s.io/apimachinery/pkg/apis/meta/v1/unstructured k8s.io/apimachinery/pkg/runtime/schema math regexp sort strconv strings time
github.com/werf/nelm/internal/resource/spec -> context fmt github.com/samber/lo github.com/werf/kubedog/pkg/trackers/rollout/multitrack github.com/werf/nelm/pkg/common github.com/werf/nelm/pkg/featgate github.com/werf/nelm/pkg/log k8s.io/apimachinery/pkg/api/meta k8s.io/apimachinery/pkg/apis/meta/v1 k8s.io/apimachinery/pkg/apis/meta/v1/unstructured k8s.io/apimachinery/pkg/runtime k8s.io/apimachinery/pkg/runtime/schema k8s.io/client-go/kubernetes/scheme regexp sort strings
github.com/werf/nelm/internal/test -> github.com/davecgh/go-spew/spew github.com/dominikbraun/graph github.com/google/go-cmp/cmp github.com/google/go-cmp/cmp/cmpopts github.com/samber/lo github.com/werf/nelm/internal/resource regexp
github.com/werf/nelm/internal/track -> context fmt github.com/chanced/caps github.com/gookit/color github.com/jedib0t/go-pretty/v6/table github.com/samber/lo github.com/werf/kubedog/pkg/trackers/dyntracker/logstore github.com/werf/kubedog/pkg/trackers/dyntracker/statestore github.com/werf/kubedog/pkg/trackers/dyntracker/util github.com/werf/nelm/pkg/log k8s.io/apimachinery/pkg/apis/meta/v1 k8s.io/apimachinery/pkg/runtime/schema sort strings time
github.com/werf/nelm/internal/util -> bufio context fmt github.com/aymanbagabas/go-udiff github.com/aymanbagabas/go-udiff/myers github.com/evanphx/json-patch github.com/gookit/color github.com/hashicorp/go-multierror github.com/looplab/fsm github.com/samber/lo github.com/tidwall/sjson github.com/wI2L/jsondiff io strings unicode
github.com/werf/nelm/pkg/action -> context encoding/json errors flag fmt github.com/Masterminds/semver/v3 github.com/alecthomas/chroma/v2 github.com/alecthomas/chroma/v2/quick github.com/alecthomas/chroma/v2/styles github.com/goccy/go-yaml github.com/gookit/color github.com/jedib0t/go-pretty/v6/table github.com/jedib0t/go-pretty/v6/text github.com/pkg/errors github.com/samber/lo github.com/werf/3p-helm/cmd/helm github.com/werf/3p-helm/pkg/action github.com/werf/3p-helm/pkg/chart/loader github.com/werf/3p-helm/pkg/chartutil github.com/werf/3p-helm/pkg/cli github.com/werf/3p-helm/pkg/kube github.com/werf/3p-helm/pkg/registry github.com/werf/3p-helm/pkg/release github.com/werf/3p-helm/pkg/storage/driver github.com/werf/3p-helm/pkg/werf/helmopts github.com/werf/common-go/pkg/secrets_manager github.com/werf/kubedog/pkg/display github.com/werf/kubedog/pkg/informer github.com/werf/kubedog/pkg/kube github.com/werf/kubedog/pkg/trackers/dyntracker/logstore github.com/werf/kubedog/pkg/trackers/dyntracker/statestore github.com/werf/kubedog/pkg/trackers/dyntracker/util github.com/werf/logboek github.com/werf/nelm/internal/chart github.com/werf/nelm/internal/kube github.com/werf/nelm/internal/legacy/deploy github.com/werf/nelm/internal/lock github.com/werf/nelm/internal/plan github.com/werf/nelm/internal/release github.com/werf/nelm/internal/resource github.com/werf/nelm/internal/resource/spec github.com/werf/nelm/internal/track github.com/werf/nelm/internal/util github.com/werf/nelm/pkg/common github.com/werf/nelm/pkg/featgate github.com/werf/nelm/pkg/legacy/secret github.com/werf/nelm/pkg/log github.com/xo/terminfo io io/ioutil k8s.io/apimachinery/pkg/api/errors k8s.io/apimachinery/pkg/apis/meta/v1 k8s.io/apimachinery/pkg/apis/meta/v1/unstructured k8s.io/apimachinery/pkg/runtime k8s.io/apimachinery/pkg/runtime/schema k8s.io/klog k8s.io/klog/v2 os path/filepath sigs.k8s.io/yaml sort strings sync time
github.com/werf/nelm/pkg/common -> fmt github.com/Masterminds/sprig/v3 github.com/docker/cli/cli/config github.com/docker/docker/pkg/homedir github.com/samber/lo github.com/werf/nelm/pkg/log k8s.io/apimachinery/pkg/apis/meta/v1 path/filepath regexp time
github.com/werf/nelm/pkg/featgate -> github.com/chanced/caps github.com/samber/lo github.com/werf/nelm/pkg/common os
github.com/werf/nelm/pkg/legacy/secret -> bytes context errors fmt github.com/google/uuid github.com/gookit/color github.com/moby/term github.com/werf/common-go/pkg/secret github.com/werf/common-go/pkg/secrets_manager github.com/werf/common-go/pkg/util github.com/werf/nelm/pkg/log golang.org/x/crypto/ssh/terminal io io/ioutil os os/exec path/filepath runtime strings
github.com/werf/nelm/pkg/log -> context flag fmt github.com/containerd/log github.com/davecgh/go-spew/spew github.com/gookit/color github.com/hofstadter-io/cinful github.com/samber/lo github.com/sirupsen/logrus github.com/werf/3p-helm/pkg/engine github.com/werf/kubedog/pkg/tracker/debug github.com/werf/kubedog/pkg/trackers/dyntracker/util github.com/werf/logboek github.com/werf/logboek/pkg/level github.com/xo/terminfo io k8s.io/klog k8s.io/klog/v2 log os slices


## Symbols

All public symbols of this repository, per package. With docs, if present. Output of go doc command.

### Package

~~~~


FUNCTIONS

func AddChartRepoConnectionFlags(cmd *cobra.Command, cfg *common.ChartRepoConnectionOptions) error
func AddKubeConnectionFlags(cmd *cobra.Command, cfg *common.KubeConnectionOptions) error
func AddSecretValuesFlags(cmd *cobra.Command, cfg *common.SecretValuesOptions) error
func AddTrackingFlags(cmd *cobra.Command, cfg *common.TrackingOptions) error
func AddValuesFlags(cmd *cobra.Command, cfg *common.ValuesOptions) error
func NewRootCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command
~~~~

### Package

~~~~
package chart // import "github.com/werf/nelm/internal/chart"


TYPES

type RenderChartOptions struct {
	common.ChartRepoConnectionOptions
	common.ValuesOptions

	ChartProvenanceKeyring  string
	ChartProvenanceStrategy string
	ChartRepoNoUpdate       bool
	ChartVersion            string
	ExtraAPIVersions        []string
	HelmOptions             helmopts.HelmOptions
	LocalKubeVersion        string
	NoStandaloneCRDs        bool
	Remote                  bool
	SubchartNotes           bool
	TemplatesAllowDNS       bool
}

type RenderChartResult struct {
	Chart         *chart.Chart
	Notes         string
	ReleaseConfig map[string]interface{}
	ResourceSpecs []*spec.ResourceSpec
	Values        map[string]interface{}
}

func RenderChart(ctx context.Context, chartPath, releaseName, releaseNamespace string, revision int, deployType common.DeployType, registryClient *registry.Client, clientFactory kube.ClientFactorier, opts RenderChartOptions) (*RenderChartResult, error)
    Download chart and its dependencies, build and merge values, then
    render templates. Most of the logic is in Helm SDK, in Nelm its mostly
    orchestration level.

~~~~

### Package

~~~~
package kube // import "github.com/werf/nelm/internal/kube"


VARIABLES

var (
	DefaultKubectlCacheDir      = filepath.Join(homedir.HomeDir(), ".kube", "cache")
	KubectlCacheDirEnv          = "KUBECACHEDIR"
	KubectlHTTPCacheSubdir      = "http"
	KubectlDiscoveryCacheSubdir = "discovery"
)

FUNCTIONS

func IsImmutableErr(err error) bool
func IsNoSuchKindErr(err error) bool
func IsNotFoundErr(err error) bool
func NewDiscoveryKubeClientFromKubeConfig(kubeConfig *KubeConfig) (*disk.CachedDiscoveryClient, error)
func NewDynamicKubeClientFromKubeConfig(kubeConfig *KubeConfig) (*dynamic.DynamicClient, error)
func NewKubeMapper(ctx context.Context, discoveryClient discovery.CachedDiscoveryInterface) meta.RESTMapper
func NewStaticKubeClientFromKubeConfig(kubeConfig *KubeConfig) (*kubernetes.Clientset, error)

TYPES

type ClientFactorier interface {
	KubeClient() KubeClienter
	Static() kubernetes.Interface
	Dynamic() dynamic.Interface
	Discovery() discovery.CachedDiscoveryInterface
	Mapper() meta.ResettableRESTMapper
	LegacyClientGetter() *LegacyClientGetter
	KubeConfig() *KubeConfig
}

var (
	AddToScheme sync.Once
)
type ClientFactory struct {
}
    Constructs all Kubernetes clients you may possibly need and makes it easy to
    pass them all around.

func NewClientFactory(ctx context.Context, kubeConfig *KubeConfig) (*ClientFactory, error)

func (f *ClientFactory) Discovery() discovery.CachedDiscoveryInterface

func (f *ClientFactory) Dynamic() dynamic.Interface

func (f *ClientFactory) KubeClient() KubeClienter

func (f *ClientFactory) KubeConfig() *KubeConfig

func (f *ClientFactory) LegacyClientGetter() *LegacyClientGetter

func (f *ClientFactory) Mapper() meta.ResettableRESTMapper

func (f *ClientFactory) Static() kubernetes.Interface

type KubeClient struct {
}
    High-level Kubernetes Client. Always prefer using it instead of
    static/dynamic Kubernetes go-client directly. Provides caching, which works
    as long as there is no other client or other program modifying Kubernetes
    resources that we work with through this client.

func NewKubeClient(staticClient kubernetes.Interface, dynamicClient dynamic.Interface, discoveryClient discovery.CachedDiscoveryInterface, mapper apimeta.ResettableRESTMapper) *KubeClient

func (c *KubeClient) Apply(ctx context.Context, resSpec *spec.ResourceSpec, opts KubeClientApplyOptions) (*unstructured.Unstructured, error)

func (c *KubeClient) Create(ctx context.Context, resSpec *spec.ResourceSpec, opts KubeClientCreateOptions) (*unstructured.Unstructured, error)

func (c *KubeClient) Delete(ctx context.Context, resMeta *spec.ResourceMeta, opts KubeClientDeleteOptions) error

func (c *KubeClient) Get(ctx context.Context, resMeta *spec.ResourceMeta, opts KubeClientGetOptions) (*unstructured.Unstructured, error)

func (c *KubeClient) MergePatch(ctx context.Context, resMeta *spec.ResourceMeta, patch []byte, opts KubeClientMergePatchOptions) (*unstructured.Unstructured, error)

type KubeClientApplyOptions struct {
	DefaultNamespace string
	DryRun           bool
}

type KubeClientCreateOptions struct {
	DefaultNamespace string
	ForceReplicas    *int
}

type KubeClientDeleteOptions struct {
	DefaultNamespace  string
	PropagationPolicy metav1.DeletionPropagation
}

type KubeClientGetOptions struct {
	DefaultNamespace string
	TryCache         bool
}

type KubeClientMergePatchOptions struct {
	DefaultNamespace string
}

type KubeClienter interface {
	Get(ctx context.Context, meta *spec.ResourceMeta, opts KubeClientGetOptions) (*unstructured.Unstructured, error)
	Create(ctx context.Context, spec *spec.ResourceSpec, opts KubeClientCreateOptions) (*unstructured.Unstructured, error)
	Apply(ctx context.Context, spec *spec.ResourceSpec, opts KubeClientApplyOptions) (*unstructured.Unstructured, error)
	MergePatch(ctx context.Context, meta *spec.ResourceMeta, patch []byte, opts KubeClientMergePatchOptions) (*unstructured.Unstructured, error)
	Delete(ctx context.Context, meta *spec.ResourceMeta, opts KubeClientDeleteOptions) error
}

type KubeConfig struct {
	LegacyClientConfig clientcmd.ClientConfig
	Namespace          string
	RawConfig          *api.Config
	RestConfig         *rest.Config
}
    Abstracts all static configuration needed to create Kubernetes clients.

func NewKubeConfig(ctx context.Context, kubeConfigPaths []string, opts KubeConfigOptions) (*KubeConfig, error)

type KubeConfigOptions struct {
	common.KubeConnectionOptions

	KubeContextNamespace string
}

type LegacyClientGetter struct {
}

func NewLegacyClientGetter(discoveryClient discovery.CachedDiscoveryInterface, mapper meta.ResettableRESTMapper, restConfig *rest.Config, legacyClientConfig clientcmd.ClientConfig) *LegacyClientGetter
    TODO(v2): get rid

func (g *LegacyClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error)

func (g *LegacyClientGetter) ToRESTConfig() (*rest.Config, error)

func (g *LegacyClientGetter) ToRESTMapper() (meta.RESTMapper, error)

func (g *LegacyClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig

~~~~

### Package

~~~~
package fake // import "github.com/werf/nelm/internal/kube/fake"


FUNCTIONS

func NewDynamicClient(staticClient *staticfake.Clientset, mapper meta.ResettableRESTMapper) *dynamicfake.FakeDynamicClient
func NewStaticClient(mapper meta.ResettableRESTMapper) *staticfake.Clientset

TYPES

type CachedDiscoveryClient struct {
	*discfake.FakeDiscovery
}

func NewCachedDiscoveryClient() (*CachedDiscoveryClient, error)

func (c *CachedDiscoveryClient) Fresh() bool

func (c *CachedDiscoveryClient) Invalidate()

type ClientFactory struct {
}

func NewClientFactory(ctx context.Context) (*ClientFactory, error)

func (f *ClientFactory) Discovery() discovery.CachedDiscoveryInterface

func (f *ClientFactory) Dynamic() dynamic.Interface

func (f *ClientFactory) KubeClient() kube.KubeClienter

func (f *ClientFactory) KubeConfig() *kube.KubeConfig

func (f *ClientFactory) LegacyClientGetter() *kube.LegacyClientGetter

func (f *ClientFactory) Mapper() meta.ResettableRESTMapper

func (f *ClientFactory) Static() kubernetes.Interface

~~~~

### Package

~~~~
package deploy // import "github.com/werf/nelm/internal/legacy/deploy"


CONSTANTS

const (
	TrackTerminationModeAnnoName = "werf.io/track-termination-mode"

	FailModeAnnoName                  = "werf.io/fail-mode"
	FailuresAllowedPerReplicaAnnoName = "werf.io/failures-allowed-per-replica"

	LogRegexAnnoName      = "werf.io/log-regex"
	LogRegexForAnnoPrefix = "werf.io/log-regex-for-"

	IgnoreReadinessProbeFailsForPrefix = "werf.io/ignore-readiness-probe-fails-for-"

	NoActivityTimeoutName = "werf.io/no-activity-timeout"

	SkipLogsAnnoName              = "werf.io/skip-logs"
	SkipLogsForContainersAnnoName = "werf.io/skip-logs-for-containers"
	ShowLogsOnlyForContainers     = "werf.io/show-logs-only-for-containers"
	ShowLogsUntilAnnoName         = "werf.io/show-logs-until"

	ShowEventsAnnoName = "werf.io/show-service-messages"

	ReplicasOnCreationAnnoName = "werf.io/replicas-on-creation"

	StageWeightAnnoName = "werf.io/weight"

	ExternalDependencyResourceAnnoName  = "external-dependency.werf.io/resource"
	ExternalDependencyNamespaceAnnoName = "external-dependency.werf.io/namespace"
)

TYPES

type ResourcesWaiter struct {
	Client                    *helm_kube.Client
	LogsFromTime              time.Time
	StatusProgressPeriod      time.Duration
	HooksStatusProgressPeriod time.Duration
}
    TODO(v2): get rid

func NewResourcesWaiter(client *helm_kube.Client, logsFromTime time.Time, statusProgressPeriod, hooksStatusProgressPeriod time.Duration) *ResourcesWaiter

func (waiter *ResourcesWaiter) Wait(ctx context.Context, resources helm_kube.ResourceList, timeout time.Duration) error

func (waiter *ResourcesWaiter) WaitUntilDeleted(ctx context.Context, specs []*helm_kube.ResourcesWaiterDeleteResourceSpec, timeout time.Duration) error

func (waiter *ResourcesWaiter) WatchUntilReady(ctx context.Context, resources helm_kube.ResourceList, timeout time.Duration) error

type StagesSplitter struct{}
    TODO(v2): get rid

func NewStagesSplitter() *StagesSplitter

func (s *StagesSplitter) Split(resources kube.ResourceList) (stages.SortedStageList, error)

~~~~

### Package

~~~~
package lock // import "github.com/werf/nelm/internal/lock"


TYPES

type ConfigMapLocker struct {
	ConfigMapName, Namespace string

	Locker lockgate.Locker

}

func NewConfigMapLocker(
	configMapName, namespace, releaseNamespace string,
	locker lockgate.Locker,
	clientFactory kube.ClientFactorier,
	options ConfigMapLockerOptions,
) *ConfigMapLocker

func (locker *ConfigMapLocker) Acquire(lockName string, opts lockgate.AcquireOptions) (
	bool,
	lockgate.LockHandle,
	error,
)

func (locker *ConfigMapLocker) Release(lock lockgate.LockHandle) error

type ConfigMapLockerOptions struct {
	CreateNamespace bool
}

type LockManager struct {
	Namespace       string
	LockerWithRetry *locker_with_retry.LockerWithRetry
}
    NOTE: LockManager for not is not multithreaded due to the lack of support of
    contexts in the lockgate library

func NewLockManager(ctx context.Context, namespace string, createNamespace bool, clientFactory kube.ClientFactorier) (*LockManager, error)

func (lockManager *LockManager) LockRelease(
	ctx context.Context,
	releaseName string,
) (lockgate.LockHandle, error)

func (lockManager *LockManager) Unlock(handle lockgate.LockHandle) error

~~~~

### Package

~~~~
package plan // import "github.com/werf/nelm/internal/plan"


CONSTANTS

const (
	HiddenVerboseCRDChanges    = "<hidden verbose CRD changes>"
	HiddenVerboseChanges       = "<hidden verbose changes>"
	HiddenInsignificantChanges = "<hidden insignificant changes>"
	HiddenSensitiveChanges     = "<hidden sensitive changes>"
)

VARIABLES

var OrderedResourceInstallTypes = []ResourceInstallType{
	ResourceInstallTypeNone,
	ResourceInstallTypeCreate,
	ResourceInstallTypeRecreate,
	ResourceInstallTypeUpdate,
	ResourceInstallTypeApply,
}

FUNCTIONS

func BuildResourceInfos(ctx context.Context, deployType common.DeployType, releaseName, releaseNamespace string, instResources []*resource.InstallableResource, delResources []*resource.DeletableResource, prevReleaseFailed bool, clientFactory kube.ClientFactorier, opts BuildResourceInfosOptions) (instResourceInfos []*InstallableResourceInfo, delResourceInfos []*DeletableResourceInfo, err error)
    From Installable/DeletableResource builds Installable/DeletableResourceInfo.
    If you can do something earlier than in BuildReleaseInfos - do it there.
    Here you can access the cluster to get more info, and here we actually
    decide what to do with each resource. Initially all this logic was in
    BuildPlan, but it became way too complex, so we extracted it here.

func ExecutePlan(parentCtx context.Context, releaseNamespace string, plan *Plan, taskStore *kdutil.Concurrent[*statestore.TaskStore], logStore *kdutil.Concurrent[*logstore.LogStore], informerFactory *kdutil.Concurrent[*informer.InformerFactory], history release.Historier, clientFactory kube.ClientFactorier, opts ExecutePlanOptions) error
    Executes the given plan. It doesn't care what kind of plan it is (install,
    upgrade, failure plan, etc.). All the differences between these plans
    must be figured out earlier, e.g. in BuildPlan. This generic design must
    be preserved. Keep it simple: if something can be done on earlier stages,
    do it there.

func InstallableResourceInfoSortByMustInstallHandler(r1, r2 *InstallableResourceInfo) bool
func InstallableResourceInfoSortByStageHandler(r1, r2 *InstallableResourceInfo) bool
func OperationID(t OperationType, version OperationVersion, iteration OperationIteration, configID string) string
func OperationIDHuman(t OperationType, iteration OperationIteration, configIDHuman string) string
func ResourceInstallTypeSortHandler(type1, type2 ResourceInstallType) bool
func ValidateRemote(releaseName, releaseNamespace string, installableResourceInfos []*InstallableResourceInfo, forceAdoption bool) error
    Should only be called if cluster access is allowed.


TYPES

type BuildFailurePlanOptions struct {
	NoFinalTracking bool
}

type BuildPlanOptions struct {
	NoFinalTracking bool
}

type BuildResourceInfosOptions struct {
	NetworkParallelism    int
	NoRemoveManualChanges bool
}

type CalculatePlannedChangesOptions struct {
	DiffContextLines       int
	ShowVerboseCRDDiffs    bool
	ShowVerboseDiffs       bool
	ShowSensitiveDiffs     bool
	ShowInsignificantDiffs bool
}

type DeletableResourceInfo struct {
	*spec.ResourceMeta

	LocalResource *resource.DeletableResource
	GetResult     *unstructured.Unstructured

	MustDelete       bool
	MustTrackAbsence bool

	Stage common.Stage
}
    A data class, which stores all info to make a decision on what to do with
    the to-be-deleted resource in the plan.

type ExecutePlanOptions struct {
	common.TrackingOptions

	NetworkParallelism int
}

type InstallableResourceInfo struct {
	*spec.ResourceMeta

	LocalResource  *resource.InstallableResource
	GetResult      *unstructured.Unstructured
	DryApplyResult *unstructured.Unstructured
	DryApplyErr    error

	MustInstall                   ResourceInstallType
	MustDeleteOnSuccessfulInstall bool
	MustDeleteOnFailedInstall     bool
	MustTrackReadiness            bool

	Stage                          common.Stage
	StageDeleteOnSuccessfulInstall common.Stage
	Iteration                      int
}
    A data class, which stores all info to make a decision on what to do with
    the to-be-installed resource in the plan.

type Operation struct {
	Type      OperationType
	Version   OperationVersion
	Category  OperationCategory
	Iteration OperationIteration
	Status    OperationStatus
	Config    OperationConfig
}
    Represents an operation on a resource, such as create, update,
    track readiness, etc. The operation ID must be unique: you can't have two
    operations with the same ID in the plan/graph. Operation must be easily
    serializable.

func (o *Operation) ID() string

func (o *Operation) IDHuman() string

type OperationCategory string

const (
	OperationCategoryMeta OperationCategory = "meta"
	OperationCategoryResource OperationCategory = "resource"
	OperationCategoryTrack OperationCategory = "track"
	OperationCategoryRelease OperationCategory = "release"
)
type OperationConfig interface {
	ID() string
	IDHuman() string
}
    Any config that is needed to execute the operation goes here, as long as it
    doesn't fit into other fields of the Operation struct. The underlying struct
    can have any number of fields of any kind, just make sure they are easily
    serializable.

type OperationConfigApply struct {
	ResourceSpec *spec.ResourceSpec
}

func (c *OperationConfigApply) ID() string

func (c *OperationConfigApply) IDHuman() string

type OperationConfigCreate struct {
	ResourceSpec  *spec.ResourceSpec
	ForceReplicas *int
}

func (c *OperationConfigCreate) ID() string

func (c *OperationConfigCreate) IDHuman() string

type OperationConfigCreateRelease struct {
	Release *helmrelease.Release
}

func (c *OperationConfigCreateRelease) ID() string

func (c *OperationConfigCreateRelease) IDHuman() string

type OperationConfigDelete struct {
	ResourceMeta      *spec.ResourceMeta
	DeletePropagation metav1.DeletionPropagation
}

func (c *OperationConfigDelete) ID() string

func (c *OperationConfigDelete) IDHuman() string

type OperationConfigDeleteRelease struct {
	ReleaseName      string
	ReleaseNamespace string
	ReleaseRevision  int
}

func (c *OperationConfigDeleteRelease) ID() string

func (c *OperationConfigDeleteRelease) IDHuman() string

type OperationConfigNoop struct {
	OpID string
}

func (c *OperationConfigNoop) ID() string

func (c *OperationConfigNoop) IDHuman() string

type OperationConfigRecreate struct {
	ResourceSpec      *spec.ResourceSpec
	DeletePropagation metav1.DeletionPropagation
	ForceReplicas     *int
}

func (c *OperationConfigRecreate) ID() string

func (c *OperationConfigRecreate) IDHuman() string

type OperationConfigTrackAbsence struct {
	ResourceMeta *spec.ResourceMeta
}

func (c *OperationConfigTrackAbsence) ID() string

func (c *OperationConfigTrackAbsence) IDHuman() string

type OperationConfigTrackPresence struct {
	ResourceMeta *spec.ResourceMeta
}

func (c *OperationConfigTrackPresence) ID() string

func (c *OperationConfigTrackPresence) IDHuman() string

type OperationConfigTrackReadiness struct {
	ResourceMeta *spec.ResourceMeta

	FailMode                                 multitrack.FailMode
	FailuresAllowed                          int
	IgnoreLogs                               bool
	IgnoreLogsForContainers                  []string
	IgnoreLogsByRegex                        *regexp.Regexp
	IgnoreLogsByRegexForContainers           map[string]*regexp.Regexp
	IgnoreReadinessProbeFailsByContainerName map[string]time.Duration
	NoActivityTimeout                        time.Duration
	SaveEvents                               bool
	SaveLogsByRegex                          *regexp.Regexp
	SaveLogsByRegexForContainers             map[string]*regexp.Regexp
	SaveLogsOnlyForContainers                []string
	SaveLogsOnlyForNumberOfReplicas          int
}

func (c *OperationConfigTrackReadiness) ID() string

func (c *OperationConfigTrackReadiness) IDHuman() string

type OperationConfigUpdate struct {
	ResourceSpec *spec.ResourceSpec
}

func (c *OperationConfigUpdate) ID() string

func (c *OperationConfigUpdate) IDHuman() string

type OperationConfigUpdateRelease struct {
	Release *helmrelease.Release
}

func (c *OperationConfigUpdateRelease) ID() string

func (c *OperationConfigUpdateRelease) IDHuman() string

type OperationIteration int
    Helps to avoid operation ID collisions. Since you can't have two operations
    with the same ID in the graph, you can increment the iteration to get a
    new unique ID for the operation. The higher the iteration, the later in the
    plan/graph the operation should appear.

type OperationStatus string

const (
	OperationStatusUnknown   OperationStatus = ""
	OperationStatusPending   OperationStatus = "pending"
	OperationStatusCompleted OperationStatus = "completed"
	OperationStatusFailed    OperationStatus = "failed"
)
type OperationType string

const (
	OperationTypeApply          OperationType = "apply"
	OperationTypeCreate         OperationType = "create"
	OperationTypeCreateRelease  OperationType = "create-release"
	OperationTypeDelete         OperationType = "delete"
	OperationTypeDeleteRelease  OperationType = "delete-release"
	OperationTypeNoop           OperationType = "noop"
	OperationTypeRecreate       OperationType = "recreate"
	OperationTypeTrackAbsence   OperationType = "track-absence"
	OperationTypeTrackPresence  OperationType = "track-presence"
	OperationTypeTrackReadiness OperationType = "track-readiness"
	OperationTypeUpdate         OperationType = "update"
	OperationTypeUpdateRelease  OperationType = "update-release"
)
type OperationVersion int
    Used to handle breaking changes in the Operation struct.

const (
	OperationVersionApply          OperationVersion = 1
	OperationVersionCreate         OperationVersion = 1
	OperationVersionCreateRelease  OperationVersion = 1
	OperationVersionDelete         OperationVersion = 1
	OperationVersionDeleteRelease  OperationVersion = 1
	OperationVersionNoop           OperationVersion = 1
	OperationVersionRecreate       OperationVersion = 1
	OperationVersionTrackAbsence   OperationVersion = 1
	OperationVersionTrackPresence  OperationVersion = 1
	OperationVersionTrackReadiness OperationVersion = 1
	OperationVersionUpdate         OperationVersion = 1
	OperationVersionUpdateRelease  OperationVersion = 1
)
type Plan struct {
	Graph graph.Graph[string, *Operation]
}
    Wrapper over dominikbraun/graph to make it easier to use as a plan/graph of
    operations.

func BuildFailurePlan(failedPlan *Plan, installableInfos []*InstallableResourceInfo, releaseInfos []*ReleaseInfo, opts BuildFailurePlanOptions) (*Plan, error)
    When the main plan fails, the failure plan must be built and executed.

func BuildPlan(installableInfos []*InstallableResourceInfo, deletableInfos []*DeletableResourceInfo, releaseInfos []*ReleaseInfo, opts BuildPlanOptions) (*Plan, error)
    Builds any kind of a plan, be it for install, upgrade, rollback or
    uninstall. The only exception is a failure plan (see BuildFailurePlan),
    because it's way too different. Any differences between different kinds
    of plans must be figured out earlier, e.g. at BuildResourceInfos level.
    This generic design must be preserved. Keep it simple: if something can be
    done on earlier stages, do it there.

func NewPlan() *Plan

func (p *Plan) AddOperationChain() *planChainBuilder

func (p *Plan) Connect(fromID, toID string) error

func (p *Plan) Operation(id string) (op *Operation, found bool)

func (p *Plan) Operations() []*Operation

func (p *Plan) Optimize(noFinalTracking bool) error

func (p *Plan) SquashOperation(op *Operation)

func (p *Plan) ToDOT() ([]byte, error)

type ReleaseInfo struct {
	Release *helmrelease.Release

	Must                   ReleaseType
	MustFailOnFailedDeploy bool
}
    Data class, which stores all info to make a decision on what to do with the
    release revision in the plan.

func BuildReleaseInfos(ctx context.Context, deployType common.DeployType, prevReleases []*helmrelease.Release, newRel *helmrelease.Release) ([]*ReleaseInfo, error)
    Build ReleaseInfos from Releases that we got from the cluster. Here we
    actually decide on what to do with each release revision. Compute here as
    much as you can: Release shouldn't be used for decision making (its just
    a JSON representation of a Helm release) and BuildPlan is complex enough
    already.

type ReleaseType string

const (
	ReleaseTypeNone ReleaseType = "none"
	ReleaseTypeInstall ReleaseType = "install"
	ReleaseTypeUpgrade ReleaseType = "upgrade"
	ReleaseTypeRollback ReleaseType = "rollback"
	ReleaseTypeSupersede ReleaseType = "supersede"
	ReleaseTypeUninstall ReleaseType = "uninstall"
	ReleaseTypeDelete ReleaseType = "delete"
)
type ResourceChange struct {
	ExtraOperations []string
	Reason       string
	ResourceMeta *spec.ResourceMeta
	Type         string
	TypeStyle    color.Style
	Udiff        string
}

func CalculatePlannedChanges(installableInfos []*InstallableResourceInfo, deletableInfos []*DeletableResourceInfo, opts CalculatePlannedChangesOptions) ([]*ResourceChange, error)
    Calculate planned changes for informational purposes. Doesn't need the full
    plan, just having Installable/DeletableResourceInfos is enough. Returns the
    structured result and shouldn't decide on how to present this data.

type ResourceInstallType string

const (
	ResourceInstallTypeNone     ResourceInstallType = "none"
	ResourceInstallTypeCreate   ResourceInstallType = "create"
	ResourceInstallTypeRecreate ResourceInstallType = "recreate"
	ResourceInstallTypeUpdate   ResourceInstallType = "update"
	ResourceInstallTypeApply ResourceInstallType = "apply"
)
~~~~

### Package

~~~~
package release // import "github.com/werf/nelm/internal/release"


FUNCTIONS

func IsReleaseUpToDate(oldRel, newRel *helmrelease.Release) (bool, error)
    Check if the new Release is up-to-date compared to the old Release.
    It doesn't check any resources of the release in the cluster, just compares
    Release objects.

func NewRelease(name, namespace string, revision int, deployType common.DeployType, resources []*spec.ResourceSpec, chart *helmchart.Chart, releaseConfig map[string]interface{}, opts ReleaseOptions) (*helmrelease.Release, error)
    Construct Helm release.

func NewReleaseStorage(ctx context.Context, namespace, storageDriver string, clientFactory kube.ClientFactorier, opts ReleaseStorageOptions) (*helmstorage.Storage, error)
    Constructs Helm release storage driver.

func ReleaseToResourceSpecs(rel *helmrelease.Release, releaseNamespace string, noCleanNullFields bool) ([]*spec.ResourceSpec, error)
    Constructs ResourceSpecs from a Release object.


TYPES

type Historier interface {
	Releases() []*helmrelease.Release
	FindAllDeployed() []*helmrelease.Release
	FindRevision(revision int) (rel *helmrelease.Release, found bool)
	CreateRelease(ctx context.Context, rel *helmrelease.Release) error
	UpdateRelease(ctx context.Context, rel *helmrelease.Release) error
	DeleteRelease(ctx context.Context, name string, revision int) error
}

type History struct {
}
    Wraps Helm release management for easier use.

func BuildHistories(historyStorage ReleaseStorager, opts HistoryOptions) ([]*History, error)
    Builds histories for multiple different releases.

func BuildHistory(releaseName string, historyStorage ReleaseStorager, opts HistoryOptions) (*History, error)
    Builds history for a specific release.

func NewHistory(rels []*helmrelease.Release, releaseName string, historyStorage ReleaseStorager, opts HistoryOptions) *History

func (h *History) CreateRelease(ctx context.Context, rel *helmrelease.Release) error

func (h *History) DeleteRelease(ctx context.Context, name string, revision int) error

func (h *History) FindAllDeployed() []*helmrelease.Release

func (h *History) FindRevision(revision int) (rel *helmrelease.Release, found bool)

func (h *History) Releases() []*helmrelease.Release

func (h *History) UpdateRelease(ctx context.Context, rel *helmrelease.Release) error

type HistoryOptions struct{}

type ReleaseOptions struct {
	InfoAnnotations map[string]string
	Labels          map[string]string
	Notes           string
}

type ReleaseStorageOptions struct {
	HistoryLimit  int
	SQLConnection string
}

type ReleaseStorager interface {
	Create(rls *helmrelease.Release) error
	Update(rls *helmrelease.Release) error
	Delete(name string, version int) (*helmrelease.Release, error)
	Query(labels map[string]string) ([]*helmrelease.Release, error)
}
    Minimal interface for Helm storage drivers.

~~~~

### Package

~~~~
package resource // import "github.com/werf/nelm/internal/resource"


CONSTANTS

const (
	HideAll = "$$HIDE_ALL$$"
)

FUNCTIONS

func BuildResources(ctx context.Context, deployType common.DeployType, releaseNamespace string, prevRelResSpecs, newRelResSpecs []*spec.ResourceSpec, patchers []spec.ResourcePatcher, clientFactory kube.ClientFactorier, opts BuildResourcesOptions) ([]*InstallableResource, []*DeletableResource, error)
    Build Installable/DeletableResources from ResourceSpecs. Resulting Resources
    can be used to construct Installable/DeletableResourceInfos later.
    Must never contact the cluster, because this is called even when no cluster
    access allowed.

func InstallableResourceSortByWeightHandler(r1, r2 *InstallableResource) bool
func IsSensitive(groupKind schema.GroupKind, annotations map[string]string) bool
func KeepOnDelete(meta *spec.ResourceMeta, releaseNamespace string) bool
func ParseSensitivePaths(value string) []string
func RedactSensitiveData(unstruct *unstructured.Unstructured, sensitivePaths []string) *unstructured.Unstructured
func ValidateLocal(releaseNamespace string, transformedResources []*InstallableResource) error
    Can be called even without cluster access.

func ValidateResourcePolicy(meta *spec.ResourceMeta) error

TYPES

type BuildResourcesOptions struct {
	Remote                   bool
	DefaultDeletePropagation metav1.DeletionPropagation
}

type DeletableResource struct {
	*spec.ResourceMeta

	Ownership         common.Ownership
	KeepOnDelete      bool
	DeletePropagation metav1.DeletionPropagation
}
    Represent a Kubernetes resource that can be deleted. Higher level than
    ResourceMeta, but lower level than DeletableResourceInfo. If something
    can be computed on this level instead of doing this on higher levels,
    it's better to do it here.

func NewDeletableResource(spec *spec.ResourceSpec, releaseNamespace string, opts DeletableResourceOptions) *DeletableResource
    Construct a DeletableResource from a ResourceSpec. Must never contact the
    cluster, because this is called even when no cluster access allowed.

type DeletableResourceOptions struct {
	DefaultDeletePropagation metav1.DeletionPropagation
}

type ExternalDependency struct {
	*spec.ResourceMeta
}
    Represents a dependency on an external resource outside of the Helm release.

type InstallableResource struct {
	*spec.ResourceSpec

	Ownership                              common.Ownership
	Recreate                               bool
	RecreateOnImmutable                    bool
	DefaultReplicasOnCreation              *int
	DeleteOnSucceeded                      bool
	DeleteOnFailed                         bool
	KeepOnDelete                           bool
	FailMode                               multitrack.FailMode
	FailuresAllowed                        int
	IgnoreReadinessProbeFailsForContainers map[string]time.Duration
	LogRegex                               *regexp.Regexp
	LogRegexesForContainers                map[string]*regexp.Regexp
	NoActivityTimeout                      time.Duration
	ShowLogsOnlyForContainers              []string
	ShowServiceMessages                    bool
	ShowLogsOnlyForNumberOfReplicas        int
	SkipLogs                               bool
	SkipLogsForContainers                  []string
	SkipLogsRegex                          *regexp.Regexp
	SkipLogsRegexForContainers             map[string]*regexp.Regexp
	TrackTerminationMode                   multitrack.TrackTerminationMode
	Weight                                 *int
	ManualInternalDependencies             []*InternalDependency
	AutoInternalDependencies               []*InternalDependency
	ExternalDependencies                   []*ExternalDependency
	DeployConditions                       map[common.On][]common.Stage
	DeletePropagation                      metav1.DeletionPropagation
}
    Represent a Kubernetes resource that can be installed. Higher level than
    ResourceSpec, but lower level than InstallableResourceInfo. If something
    can be computed on this level instead of doing this on higher levels,
    it's better to do it here.

func NewInstallableResource(res *spec.ResourceSpec, releaseNamespace string, clientFactory kube.ClientFactorier, opts InstallableResourceOptions) (*InstallableResource, error)
    Construct an InstallableResource from a ResourceSpec. Must never contact the
    cluster, because this is called even when no cluster access allowed.

type InstallableResourceOptions struct {
	Remote                   bool
	DefaultDeletePropagation metav1.DeletionPropagation
}

type InternalDependency struct {
	*spec.ResourceMatcher

	ResourceState common.ResourceState
}
    Represents a dependency on a Kubernetes resource in the Helm release.

type SensitiveInfo struct {
	IsSensitive    bool
	SensitivePaths []string
}

func GetSensitiveInfo(groupKind schema.GroupKind, annotations map[string]string) SensitiveInfo

func (i *SensitiveInfo) FullySensitive() bool

~~~~

### Package

~~~~
package spec // import "github.com/werf/nelm/internal/resource/spec"


FUNCTIONS

func CleanUnstruct(unstruct *unstructured.Unstructured, opts CleanUnstructOptions) *unstructured.Unstructured
    Clean an Unstructured object from things like managed fields,
    non-deterministic runtime data, etc, for diffing, hashing, human-friendly
    output and so on.

func FindAnnotationOrLabelByKeyPattern(annotationsOrLabels map[string]string, pattern *regexp.Regexp) (key, value string, found bool)
func FindAnnotationsOrLabelsByKeyPattern(annotationsOrLabels map[string]string, pattern *regexp.Regexp) (result map[string]string, found bool)
func GVKtoGVR(gvk schema.GroupVersionKind, mapper meta.RESTMapper) (gvr schema.GroupVersionResource, namespaced bool, err error)
func GVRtoGVK(gvr schema.GroupVersionResource, restMapper meta.RESTMapper) (schema.GroupVersionKind, error)
func ID(name, namespace, group, kind string) string
func IDHuman(name, namespace, group, kind string) string
func IDWithVersion(name, namespace, group, version, kind string) string
func IsCRD(groupKind schema.GroupKind) bool
func IsCRDFromGR(groupKind schema.GroupResource) bool
func IsHook(annotations map[string]string) bool
func IsReleaseNamespace(resourceName string, resourceGVK schema.GroupVersionKind, releaseNamespace string) bool
func IsWebhook(groupKind schema.GroupKind) bool
func Namespaced(gvk schema.GroupVersionKind, mapper meta.RESTMapper) (bool, error)
func ParseKubectlResourceStringToGVR(resource string) schema.GroupVersionResource
func ParseKubectlResourceStringtoGVK(resource string, restMapper meta.RESTMapper) (schema.GroupVersionKind, error)
func ResourceMetaSortHandler(r1, r2 *ResourceMeta) bool
func ResourceSpecSortHandler(r1, r2 *ResourceSpec) bool

TYPES

type CleanUnstructOptions struct {
	CleanHelmShAnnos        bool
	CleanManagedFields      bool
	CleanNullFields         bool
	CleanReleaseAnnosLabels bool
	CleanRuntimeData        bool
	CleanWerfIoAnnos        bool
	CleanWerfIoRuntimeAnnos bool
}

type DropInvalidAnnotationsAndLabelsTransformer struct{}
    TODO(v2): remove this transformer. Replace it with proper early validation
    of resource Heads.

func NewDropInvalidAnnotationsAndLabelsTransformer() *DropInvalidAnnotationsAndLabelsTransformer

func (t *DropInvalidAnnotationsAndLabelsTransformer) Match(ctx context.Context, info *ResourceTransformerResourceInfo) (matched bool, err error)

func (t *DropInvalidAnnotationsAndLabelsTransformer) Transform(ctx context.Context, info *ResourceTransformerResourceInfo) ([]*unstructured.Unstructured, error)

func (t *DropInvalidAnnotationsAndLabelsTransformer) Type() ResourceTransformerType

type ExtraMetadataPatcher struct {
}

func NewExtraMetadataPatcher(annotations, labels map[string]string) *ExtraMetadataPatcher

func (p *ExtraMetadataPatcher) Match(ctx context.Context, info *ResourcePatcherResourceInfo) (bool, error)

func (p *ExtraMetadataPatcher) Patch(ctx context.Context, info *ResourcePatcherResourceInfo) (*unstructured.Unstructured, error)

func (p *ExtraMetadataPatcher) Type() ResourcePatcherType

type LegacyOnlyTrackJobsPatcher struct{}
    TODO(v2): get rid of it when patching is implemented or when Kubedog
    compaitiblity with Helm charts improved

func NewLegacyOnlyTrackJobsPatcher() *LegacyOnlyTrackJobsPatcher

func (p *LegacyOnlyTrackJobsPatcher) Match(ctx context.Context, info *ResourcePatcherResourceInfo) (bool, error)

func (p *LegacyOnlyTrackJobsPatcher) Patch(ctx context.Context, info *ResourcePatcherResourceInfo) (*unstructured.Unstructured, error)

func (p *LegacyOnlyTrackJobsPatcher) Type() ResourcePatcherType

type ReleaseMetadataPatcher struct {
}

func NewReleaseMetadataPatcher(releaseName, releaseNamespace string) *ReleaseMetadataPatcher

func (p *ReleaseMetadataPatcher) Match(ctx context.Context, info *ResourcePatcherResourceInfo) (bool, error)

func (p *ReleaseMetadataPatcher) Patch(ctx context.Context, info *ResourcePatcherResourceInfo) (*unstructured.Unstructured, error)

func (p *ReleaseMetadataPatcher) Type() ResourcePatcherType

type ResourceListsTransformer struct{}

func NewResourceListsTransformer() *ResourceListsTransformer

func (t *ResourceListsTransformer) Match(ctx context.Context, info *ResourceTransformerResourceInfo) (matched bool, err error)

func (t *ResourceListsTransformer) Transform(ctx context.Context, info *ResourceTransformerResourceInfo) ([]*unstructured.Unstructured, error)

func (t *ResourceListsTransformer) Type() ResourceTransformerType

type ResourceMatcher struct {
	Names      []string
	Namespaces []string
	Groups     []string
	Versions   []string
	Kinds      []string
}

func (s *ResourceMatcher) Match(resMeta *ResourceMeta) bool

type ResourceMeta struct {
	Name             string
	Namespace        string
	GroupVersionKind schema.GroupVersionKind
	FilePath         string
	Annotations      map[string]string
	Labels           map[string]string
}
    Contains basic information about a Kubernetes resource, without its full
    spec. Very useful for getting, deleting, tracking resources, when you don't
    care about the resource spec (or when it's not available). If also the
    resource spec is needed, use ResourceSpec.

func NewResourceMeta(name, namespace, releaseNamespace, filePath string, gvk schema.GroupVersionKind, annotations, labels map[string]string) *ResourceMeta

func NewResourceMetaFromManifest(manifest, releaseNamespace string) (*ResourceMeta, error)

func NewResourceMetaFromPartialMetadata(meta *v1.PartialObjectMetadata, releaseNamespace, filePath string) *ResourceMeta

func NewResourceMetaFromUnstructured(unstruct *unstructured.Unstructured, releaseNamespace, filePath string) *ResourceMeta

func (m *ResourceMeta) ID() string
    Uniquely identifies the resource.

func (m *ResourceMeta) IDHuman() string

func (m *ResourceMeta) IDWithVersion() string

type ResourcePatcher interface {
	Match(ctx context.Context, resourceInfo *ResourcePatcherResourceInfo) (matched bool, err error)
	Patch(ctx context.Context, matchedResourceInfo *ResourcePatcherResourceInfo) (output *unstructured.Unstructured, err error)
	Type() ResourcePatcherType
}

type ResourcePatcherResourceInfo struct {
	Obj       *unstructured.Unstructured
	Ownership common.Ownership
}

type ResourcePatcherType string

const (
	TypeExtraMetadataPatcher   ResourcePatcherType = "extra-metadata-patcher"
	TypeReleaseMetadataPatcher ResourcePatcherType = "release-metadata-patcher"
	TypeOnlyTrackJobsPatcher   ResourcePatcherType = "only-track-jobs-patcher"
)
type ResourceSpec struct {
	*ResourceMeta

	Unstruct *unstructured.Unstructured
	StoreAs  common.StoreAs
}
    Contains all generic information about the resource, e.g. its name,
    namespace, GVK and its spec. Enough to create or update resources, as well
    as delete, get, etc. Use it when ResourceMeta is not enough and you also
    need the actual resource spec.

func BuildReleasableResourceSpecs(ctx context.Context, releaseNamespace string, transformedResources []*ResourceSpec, patchers []ResourcePatcher) ([]*ResourceSpec, error)
    Patch ResourceSpecs to make them releasable, after which they can be
    saved into the Helm release. Don't try to add/delete/expand specs here,
    use transformers in BuildTransformedResourceSpecs instead.

func BuildTransformedResourceSpecs(ctx context.Context, releaseNamespace string, resources []*ResourceSpec, transformers []ResourceTransformer) ([]*ResourceSpec, error)
    Transforms ResourceSpecs, which means specs can be added, deleted,
    expanded (like Lists). If you just need to modify specs, use patchers in
    BuildReleasableResourceSpecs instead.

func NewResourceSpec(unstruct *unstructured.Unstructured, releaseNamespace string, opts ResourceSpecOptions) *ResourceSpec

func NewResourceSpecFromManifest(manifest, releaseNamespace string, opts ResourceSpecOptions) (*ResourceSpec, error)

func (s *ResourceSpec) SetAnnotations(annotations map[string]string)

func (s *ResourceSpec) SetLabels(labels map[string]string)

type ResourceSpecOptions struct {
	FilePath                string
	LegacyNoCleanNullFields bool // TODO(v2): always clean
	StoreAs                 common.StoreAs
}

type ResourceTransformer interface {
	Match(ctx context.Context, resourceInfo *ResourceTransformerResourceInfo) (matched bool, err error)
	Transform(ctx context.Context, matchedResourceInfo *ResourceTransformerResourceInfo) (output []*unstructured.Unstructured, err error)
	Type() ResourceTransformerType
}

type ResourceTransformerResourceInfo struct {
	Obj *unstructured.Unstructured
}

type ResourceTransformerType string

const (
	TypeDropInvalidAnnotationsAndLabelsTransformer ResourceTransformerType = "drop-invalid-annotations-and-labels-transformer"
	TypeResourceListsTransformer                   ResourceTransformerType = "resource-lists-transformer"
)
~~~~

### Package

~~~~
package test // import "github.com/werf/nelm/internal/test"


FUNCTIONS

func CompareInternalDependencyOption() cmp.Option
func CompareRegexpOption() cmp.Option
func CompareResourceMetadataOption(releaseNamespace string) cmp.Option
func IgnoreEdgeOption() cmp.Option
~~~~

### Package

~~~~
package track // import "github.com/werf/nelm/internal/track"


TYPES

type ProgressTablesPrinter struct {
}
    Prints progress tables at regular intervals. Progress tables include
    resource statuses, container logs and Kubernetes events.

func NewProgressTablesPrinter(taskStore *kdutil.Concurrent[*statestore.TaskStore], logStore *kdutil.Concurrent[*logstore.LogStore], opts ProgressTablesPrinterOptions) *ProgressTablesPrinter

func (p *ProgressTablesPrinter) Start(ctx context.Context, interval time.Duration)

func (p *ProgressTablesPrinter) Stop()

func (p *ProgressTablesPrinter) Wait()

type ProgressTablesPrinterOptions struct {
	DefaultNamespace string
}

~~~~

### Package

~~~~
package util // import "github.com/werf/nelm/internal/util"


FUNCTIONS

func Capitalize(s string) string
func ColoredUnifiedDiff(from, to string, diffContextLines int) string
func JSONPatchPathToJSONPath(path string) string
func MergeJSON(mergeA, toB []byte) (result []byte, changed bool, err error)
func Multierrorf(format string, errs []error, a ...any) error
func ParseProperties(ctx context.Context, input string) (map[string]any, error)
    This is a pretty well-thought-out properties parser using a finite state
    machine (FSM). We should use it more, especially in annotations.

func SubtractJSON(fromA, subtractB []byte) (result []byte, changed bool, err error)
func Uint64ToInt(v uint64) int
~~~~

### Package

~~~~
package action // import "github.com/werf/nelm/pkg/action"


CONSTANTS

const (
	DefaultReleaseGetOutputFormat = common.OutputFormatYAML
	DefaultReleaseGetLogLevel     = log.ErrorLevel
)
const (
	DefaultReleaseListOutputFormat = common.OutputFormatTable
	DefaultReleaseListLogLevel     = log.ErrorLevel
)
const (
	DefaultVersionOutputFormat = common.OutputFormatYAML
	DefaultVersionLogLevel     = log.ErrorLevel
)
const (
	DefaultChartLintLogLevel = log.InfoLevel
)
const (
	DefaultChartRenderLogLevel = log.ErrorLevel
)
const (
	DefaultLegacyReleaseUninstallLogLevel = log.InfoLevel
)
const (
	DefaultReleaseInstallLogLevel = log.InfoLevel
)
const (
	DefaultReleasePlanInstallLogLevel = log.InfoLevel
)
const (
	DefaultReleaseRollbackLogLevel = log.InfoLevel
)
const (
	DefaultReleaseUninstallLogLevel = log.InfoLevel
)
const (
	DefaultSecretFileDecryptLogLevel = log.ErrorLevel
)
const (
	DefaultSecretFileEditLogLevel = log.ErrorLevel
)
const (
	DefaultSecretFileEncryptLogLevel = log.ErrorLevel
)
const (
	DefaultSecretKeyCreateLogLevel = log.ErrorLevel
)
const (
	DefaultSecretKeyRotateLogLevel = log.InfoLevel
)
const (
	DefaultSecretValuesFileDecryptLogLevel = log.ErrorLevel
)
const (
	DefaultSecretValuesFileEditLogLevel = log.ErrorLevel
)
const (
	DefaultSecretValuesFileEncryptLogLevel = log.ErrorLevel
)

VARIABLES

var (
	ErrChangesPlanned         = errors.New("changes planned")
	ErrResourceChangesPlanned = errors.New("resource changes planned")
	ErrReleaseInstallPlanned  = errors.New("no resource changes planned, but still must install release")
)
    TODO(v2): get rid


FUNCTIONS

func ChartLint(ctx context.Context, opts ChartLintOptions) error
    Lint the Helm chart.

func LegacyReleaseUninstall(ctx context.Context, releaseName, releaseNamespace string, opts LegacyReleaseUninstallOptions) error
func ReleaseInstall(ctx context.Context, releaseName, releaseNamespace string, opts ReleaseInstallOptions) error
func ReleasePlanInstall(ctx context.Context, releaseName, releaseNamespace string, opts ReleasePlanInstallOptions) error
    Plans the next release installation without applying changes to the cluster.

func ReleaseRollback(ctx context.Context, releaseName, releaseNamespace string, opts ReleaseRollbackOptions) error
    Rolls back the Helm release to the specified revision.

func ReleaseUninstall(ctx context.Context, releaseName, releaseNamespace string, opts ReleaseUninstallOptions) error
    Uninstall the Helm release along with its resources from the cluster.

func SecretFileDecrypt(ctx context.Context, filePath string, opts SecretFileDecryptOptions) error
func SecretFileEdit(ctx context.Context, filePath string, opts SecretFileEditOptions) error
func SecretFileEncrypt(ctx context.Context, filePath string, opts SecretFileEncryptOptions) error
func SecretKeyCreate(ctx context.Context, opts SecretKeyCreateOptions) (string, error)
func SecretKeyRotate(ctx context.Context, opts SecretKeyRotateOptions) error
func SecretValuesFileDecrypt(ctx context.Context, valuesFilePath string, opts SecretValuesFileDecryptOptions) error
func SecretValuesFileEdit(ctx context.Context, valuesFilePath string, opts SecretValuesFileEditOptions) error
func SecretValuesFileEncrypt(ctx context.Context, valuesFilePath string, opts SecretValuesFileEncryptOptions) error

TYPES

type ChartLintOptions struct {
	common.KubeConnectionOptions
	common.ChartRepoConnectionOptions
	common.ValuesOptions
	common.SecretValuesOptions

	Chart string
	ChartAppVersion string
	ChartDirPath string // TODO(v2): get rid
	ChartProvenanceKeyring string
	ChartProvenanceStrategy string
	ChartRepoSkipUpdate bool
	ChartVersion string
	DefaultChartAPIVersion string
	DefaultChartName string
	DefaultChartVersion string
	DefaultDeletePropagation string
	ExtraAPIVersions []string
	ExtraAnnotations map[string]string
	ExtraLabels map[string]string
	ExtraRuntimeAnnotations map[string]string
	ExtraRuntimeLabels map[string]string
	ForceAdoption bool
	LegacyChartType helmopts.ChartType
	LegacyExtraValues map[string]interface{}
	LegacyLogRegistryStreamOut io.Writer
	LocalKubeVersion string
	NetworkParallelism int
	NoFinalTracking bool
	NoRemoveManualChanges bool
	RegistryCredentialsPath string
	ReleaseName string
	ReleaseNamespace string
	ReleaseStorageDriver string
	ReleaseStorageSQLConnection string
	Remote bool
	TempDirPath string
	TemplatesAllowDNS bool
}

type ChartRenderOptions struct {
	common.KubeConnectionOptions
	common.ChartRepoConnectionOptions
	common.ValuesOptions
	common.SecretValuesOptions

	Chart string
	ChartAppVersion string
	ChartDirPath string // TODO(v2): get rid
	ChartProvenanceKeyring string
	ChartProvenanceStrategy string
	ChartRepoSkipUpdate bool
	ChartVersion string
	DefaultChartAPIVersion string
	DefaultChartName string
	DefaultChartVersion string
	ExtraAPIVersions []string
	ExtraAnnotations map[string]string
	ExtraLabels map[string]string
	ExtraRuntimeAnnotations map[string]string // TODO(v2): get rid?? or do custom logic
	ForceAdoption bool // TODO(v2): get rid, useless
	LegacyChartType helmopts.ChartType
	LegacyExtraValues map[string]interface{}
	LegacyLogRegistryStreamOut io.Writer
	LocalKubeVersion string
	NetworkParallelism int
	OutputFilePath string
	OutputNoPrint bool
	RegistryCredentialsPath string
	ReleaseName string
	ReleaseNamespace string
	ReleaseStorageDriver string
	ReleaseStorageSQLConnection string
	Remote bool
	ShowOnlyFiles []string
	ShowStandaloneCRDs bool
	TempDirPath string
	TemplatesAllowDNS bool
}

type ChartRenderResultV2 struct {
	APIVersion string               `json:"apiVersion,omitempty"`
	Resources  []*spec.ResourceSpec `json:"resources,omitempty"`
}

func ChartRender(ctx context.Context, opts ChartRenderOptions) (*ChartRenderResultV2, error)
    Render the Helm chart.

type LegacyReleaseUninstallOptions struct {
	common.KubeConnectionOptions
	common.TrackingOptions

	NoDeleteHooks          bool
	DeleteReleaseNamespace bool
	NetworkParallelism     int
	ReleaseHistoryLimit    int
	ReleaseStorageDriver   string
	TempDirPath            string
	Timeout                time.Duration
}

type ReleaseGetOptions struct {
	common.KubeConnectionOptions

	NetworkParallelism int
	OutputFormat string
	OutputNoPrint bool
	PrintValues bool
	ReleaseStorageDriver string
	ReleaseStorageSQLConnection string
	Revision int
	TempDirPath string
}

type ReleaseGetResultChart struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	AppVersion string `json:"appVersion"`
}

type ReleaseGetResultDeployedAt struct {
	Human string `json:"human"`
	Unix  int    `json:"unix"`
}
    TODO(v2): get rid

type ReleaseGetResultRelease struct {
	Name          string                      `json:"name"`
	Namespace     string                      `json:"namespace"`
	Revision      int                         `json:"revision"`
	Status        helmrelease.Status          `json:"status"`
	DeployedAt    *ReleaseGetResultDeployedAt `json:"deployedAt"`
	Annotations   map[string]string           `json:"annotations"`
	StorageLabels map[string]string           `json:"storageLabels"`
}

type ReleaseGetResultV1 struct {
	APIVersion string                   `json:"apiVersion"`
	Release    *ReleaseGetResultRelease `json:"release"`
	Chart      *ReleaseGetResultChart   `json:"chart"`
	Notes      string                   `json:"notes,omitempty"`
	Values     map[string]interface{}   `json:"values,omitempty"`
	Hooks     []map[string]interface{} `json:"hooks,omitempty"`
	Resources []map[string]interface{} `json:"resources,omitempty"`
}

func ReleaseGet(ctx context.Context, releaseName, releaseNamespace string, opts ReleaseGetOptions) (*ReleaseGetResultV1, error)
    Retrieves detailed information about the Helm release from the cluster.

type ReleaseInstallOptions struct {
	common.KubeConnectionOptions
	common.ChartRepoConnectionOptions
	common.ValuesOptions
	common.SecretValuesOptions
	common.TrackingOptions

	AutoRollback bool
	Chart string
	ChartAppVersion string
	ChartDirPath string // TODO(v2): get rid
	ChartProvenanceKeyring string
	ChartProvenanceStrategy string
	ChartRepoSkipUpdate bool
	ChartVersion string
	DefaultChartAPIVersion string
	DefaultChartName string
	DefaultChartVersion string
	DefaultDeletePropagation string
	ExtraAnnotations map[string]string
	ExtraLabels map[string]string
	ExtraRuntimeAnnotations map[string]string
	ExtraRuntimeLabels map[string]string
	ForceAdoption bool
	InstallGraphPath string
	InstallReportPath string
	LegacyChartType helmopts.ChartType
	LegacyExtraValues map[string]interface{}
	LegacyLogRegistryStreamOut io.Writer
	NetworkParallelism int
	NoInstallStandaloneCRDs bool
	NoRemoveManualChanges bool
	NoShowNotes bool
	RegistryCredentialsPath string
	ReleaseHistoryLimit int
	ReleaseInfoAnnotations map[string]string
	ReleaseLabels map[string]string
	ReleaseStorageDriver string
	ReleaseStorageSQLConnection string
	RollbackGraphPath string
	ShowSubchartNotes bool
	TempDirPath string
	TemplatesAllowDNS bool
	Timeout time.Duration
}

type ReleaseListOptions struct {
	common.KubeConnectionOptions

	NetworkParallelism int
	OutputFormat string
	OutputNoPrint bool
	ReleaseNamespace string
	ReleaseStorageDriver string
	ReleaseStorageSQLConnection string
	TempDirPath string
}

type ReleaseListResultChart struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	AppVersion string `json:"appVersion"`
}

type ReleaseListResultDeployedAt struct {
	Human string `json:"human"`
	Unix  int    `json:"unix"`
}
    TODO(v2): get rid

type ReleaseListResultRelease struct {
	Name        string                       `json:"name"`
	Namespace   string                       `json:"namespace"`
	Revision    int                          `json:"revision"`
	Status      helmrelease.Status           `json:"status"`
	DeployedAt  *ReleaseListResultDeployedAt `json:"deployedAt"`
	Annotations map[string]string            `json:"annotations"`
	Chart       *ReleaseListResultChart      `json:"chart"`
}

type ReleaseListResultV1 struct {
	APIVersion string                      `json:"apiVersion"`
	Releases   []*ReleaseListResultRelease `json:"releases"`
}

func ReleaseList(ctx context.Context, opts ReleaseListOptions) (*ReleaseListResultV1, error)
    Lists Helm releases from the cluster.

type ReleaseNotFoundError struct {
	ReleaseName      string
	ReleaseNamespace string
}

func (e *ReleaseNotFoundError) Error() string

type ReleasePlanInstallOptions struct {
	common.KubeConnectionOptions
	common.ChartRepoConnectionOptions
	common.ValuesOptions
	common.SecretValuesOptions

	Chart string
	ChartAppVersion string
	ChartDirPath string // TODO(v2): get rid
	ChartProvenanceKeyring string
	ChartProvenanceStrategy string
	ChartRepoSkipUpdate bool
	ChartVersion string
	DefaultChartAPIVersion string
	DefaultChartName string
	DefaultChartVersion string
	DefaultDeletePropagation string
	DiffContextLines int
	ErrorIfChangesPlanned bool
	ExtraAnnotations map[string]string
	ExtraLabels map[string]string
	ExtraRuntimeAnnotations map[string]string
	ExtraRuntimeLabels map[string]string
	ForceAdoption bool
	InstallGraphPath string
	LegacyChartType helmopts.ChartType
	LegacyExtraValues map[string]interface{}
	LegacyHelmCompatibleTracking bool
	LegacyLogRegistryStreamOut io.Writer
	NetworkParallelism int
	NoFinalTracking bool
	NoInstallStandaloneCRDs bool
	NoRemoveManualChanges bool
	RegistryCredentialsPath string
	ReleaseInfoAnnotations map[string]string
	ReleaseLabels map[string]string
	ReleaseStorageDriver string
	ReleaseStorageSQLConnection string
	ShowInsignificantDiffs bool
	ShowSensitiveDiffs bool
	ShowVerboseCRDDiffs bool
	ShowVerboseDiffs bool
	TempDirPath string
	TemplatesAllowDNS bool
	Timeout time.Duration
}

type ReleaseRevisionNotFoundError struct {
	ReleaseName      string
	ReleaseNamespace string
	Revision         int
}

func (e *ReleaseRevisionNotFoundError) Error() string

type ReleaseRollbackOptions struct {
	common.KubeConnectionOptions
	common.TrackingOptions

	DefaultDeletePropagation string
	ExtraRuntimeAnnotations map[string]string
	ExtraRuntimeLabels map[string]string
	ForceAdoption bool
	NetworkParallelism int
	NoRemoveManualChanges bool
	NoShowNotes bool
	ReleaseHistoryLimit int
	ReleaseInfoAnnotations map[string]string
	ReleaseLabels map[string]string
	ReleaseStorageDriver string
	ReleaseStorageSQLConnection string
	Revision int
	RollbackGraphPath string
	RollbackReportPath string
	TempDirPath string
	Timeout time.Duration
}

type ReleaseUninstallOptions struct {
	common.KubeConnectionOptions
	common.TrackingOptions

	DefaultDeletePropagation string
	DeleteReleaseNamespace bool
	NetworkParallelism int
	NoRemoveManualChanges bool
	ReleaseHistoryLimit int
	ReleaseStorageDriver string
	ReleaseStorageSQLConnection string
	TempDirPath string
	Timeout time.Duration
	UninstallGraphPath string
	UninstallReportPath string
}

type SecretFileDecryptOptions struct {
	OutputFilePath string
	SecretKey      string
	SecretWorkDir  string
	TempDirPath    string
}

type SecretFileEditOptions struct {
	SecretKey     string
	SecretWorkDir string
	TempDirPath   string
}

type SecretFileEncryptOptions struct {
	OutputFilePath string
	SecretKey      string
	SecretWorkDir  string
	TempDirPath    string
}

type SecretKeyCreateOptions struct {
	OutputNoPrint bool
	TempDirPath   string
}

type SecretKeyRotateOptions struct {
	ChartDirPath      string
	NewSecretKey      string
	OldSecretKey      string
	SecretValuesFiles []string
	SecretWorkDir     string
	TempDirPath       string
}

type SecretValuesFileDecryptOptions struct {
	OutputFilePath string
	SecretKey      string
	SecretWorkDir  string
	TempDirPath    string
}

type SecretValuesFileEditOptions struct {
	SecretKey     string
	SecretWorkDir string
	TempDirPath   string
}

type SecretValuesFileEncryptOptions struct {
	OutputFilePath string
	SecretKey      string
	SecretWorkDir  string
	TempDirPath    string
}

type VersionOptions struct {
	OutputFormat string
	OutputNoPrint bool
	TempDirPath string
}

type VersionResult struct {
	FullVersion  string `json:"full"`
	MajorVersion int    `json:"major"`
	MinorVersion int    `json:"minor"`
	PatchVersion int    `json:"patch"`
}

func Version(ctx context.Context, opts VersionOptions) (*VersionResult, error)

~~~~

### Package

~~~~
package common // import "github.com/werf/nelm/pkg/common"


CONSTANTS

const (
	DefaultBurstLimit = 100
	DefaultChartProvenanceStrategy = "never"
	DefaultDeletePropagation = metav1.DeletePropagationForeground
	DefaultDiffContextLines  = 3
	DefaultFieldManager      = "helm"
	DefaultLocalKubeVersion      = "1.20.0"
	DefaultLogColorMode          = log.LogColorModeAuto
	DefaultNetworkParallelism    = 30
	DefaultProgressPrintInterval = 5 * time.Second
	DefaultQPSLimit              = 30
	DefaultReleaseHistoryLimit   = 10
	KubectlEditFieldManager      = "kubectl-edit"
	OldFieldManagerPrefix        = "werf"
	StageEndSuffix               = "end"
	StagePrefix                  = "stage"
	StageStartSuffix             = "start"
	StubReleaseName              = "stub-release"
	StubReleaseNamespace         = "stub-namespace"
)
const (
	OutputFormatJSON  = "json"
	OutputFormatTable = "table"
	OutputFormatYAML  = "yaml"
)
const (
	ReleaseStorageDriverConfigMap  = "configmap"
	ReleaseStorageDriverConfigMaps = "configmaps"
	ReleaseStorageDriverDefault    = ""
	ReleaseStorageDriverMemory     = "memory"
	ReleaseStorageDriverSQL        = "sql"
	ReleaseStorageDriverSecret     = "secret"
	ReleaseStorageDriverSecrets    = "secrets"
)

VARIABLES

var (
	Brand   = "Nelm"
	Version = "0.0.0"
)
var (
	LabelKeyHumanManagedBy   = "app.kubernetes.io/managed-by"
	LabelKeyPatternManagedBy = regexp.MustCompile(`^app.kubernetes.io/managed-by$`)

	AnnotationKeyHumanReleaseName   = "meta.helm.sh/release-name"
	AnnotationKeyPatternReleaseName = regexp.MustCompile(`^meta.helm.sh/release-name$`)

	AnnotationKeyHumanReleaseNamespace   = "meta.helm.sh/release-namespace"
	AnnotationKeyPatternReleaseNamespace = regexp.MustCompile(`^meta.helm.sh/release-namespace$`)

	AnnotationKeyHumanHook   = "helm.sh/hook"
	AnnotationKeyPatternHook = regexp.MustCompile(`^helm.sh/hook$`)

	AnnotationKeyHumanResourcePolicy   = "helm.sh/resource-policy"
	AnnotationKeyPatternResourcePolicy = regexp.MustCompile(`^helm.sh/resource-policy$`)

	AnnotationKeyHumanDeletePolicy   = "werf.io/delete-policy"
	AnnotationKeyPatternDeletePolicy = regexp.MustCompile(`^werf.io/delete-policy$`)

	AnnotationKeyHumanHookDeletePolicy   = "helm.sh/hook-delete-policy"
	AnnotationKeyPatternHookDeletePolicy = regexp.MustCompile(`^helm.sh/hook-delete-policy$`)

	AnnotationKeyHumanReplicasOnCreation   = "werf.io/replicas-on-creation"
	AnnotationKeyPatternReplicasOnCreation = regexp.MustCompile(`^werf.io/replicas-on-creation$`)

	AnnotationKeyHumanFailMode   = "werf.io/fail-mode"
	AnnotationKeyPatternFailMode = regexp.MustCompile(`^werf.io/fail-mode$`)

	AnnotationKeyHumanFailuresAllowedPerReplica   = "werf.io/failures-allowed-per-replica"
	AnnotationKeyPatternFailuresAllowedPerReplica = regexp.MustCompile(`^werf.io/failures-allowed-per-replica$`)

	AnnotationKeyHumanIgnoreReadinessProbeFailsFor   = "werf.io/ignore-readiness-probe-fails-for-<container>"
	AnnotationKeyPatternIgnoreReadinessProbeFailsFor = regexp.MustCompile(`^werf.io/ignore-readiness-probe-fails-for-(?P<container>.+)$`)

	AnnotationKeyHumanLogRegex   = "werf.io/log-regex"
	AnnotationKeyPatternLogRegex = regexp.MustCompile(`^werf.io/log-regex$`)

	AnnotationKeyHumanLogRegexSkip   = "werf.io/log-regex-skip"
	AnnotationKeyPatternLogRegexSkip = regexp.MustCompile(`^werf.io/log-regex-skip$`)

	AnnotationKeyHumanLogRegexFor   = "werf.io/log-regex-for-<container>"
	AnnotationKeyPatternLogRegexFor = regexp.MustCompile(`^werf.io/log-regex-for-(?P<container>.+)$`)

	AnnotationKeyHumanSkipLogRegexFor   = "werf.io/log-regex-skip-for-<container>"
	AnnotationKeyPatternSkipLogRegexFor = regexp.MustCompile(`^werf.io/log-regex-skip-for-(?P<container>.+)$`)

	AnnotationKeyHumanNoActivityTimeout   = "werf.io/no-activity-timeout"
	AnnotationKeyPatternNoActivityTimeout = regexp.MustCompile(`^werf.io/no-activity-timeout$`)

	AnnotationKeyHumanShowLogsOnlyForContainers   = "werf.io/show-logs-only-for-containers"
	AnnotationKeyPatternShowLogsOnlyForContainers = regexp.MustCompile(`^werf.io/show-logs-only-for-containers$`)

	AnnotationKeyHumanShowServiceMessages   = "werf.io/show-service-messages"
	AnnotationKeyPatternShowServiceMessages = regexp.MustCompile(`^werf.io/show-service-messages$`)

	AnnotationKeyHumanShowLogsOnlyForNumberOfReplicas   = "werf.io/show-logs-only-for-number-of-replicas"
	AnnotationKeyPatternShowLogsOnlyForNumberOfReplicas = regexp.MustCompile(`^werf.io/show-logs-only-for-number-of-replicas$`)

	AnnotationKeyHumanSkipLogs   = "werf.io/skip-logs"
	AnnotationKeyPatternSkipLogs = regexp.MustCompile(`^werf.io/skip-logs$`)

	AnnotationKeyHumanSkipLogsForContainers   = "werf.io/skip-logs-for-containers"
	AnnotationKeyPatternSkipLogsForContainers = regexp.MustCompile(`^werf.io/skip-logs-for-containers$`)

	AnnotationKeyHumanTrackTerminationMode   = "werf.io/track-termination-mode"
	AnnotationKeyPatternTrackTerminationMode = regexp.MustCompile(`^werf.io/track-termination-mode$`)

	AnnotationKeyHumanWeight   = "werf.io/weight"
	AnnotationKeyPatternWeight = regexp.MustCompile(`^werf.io/weight$`)

	AnnotationKeyHumanHookWeight   = "helm.sh/hook-weight"
	AnnotationKeyPatternHookWeight = regexp.MustCompile(`^helm.sh/hook-weight$`)

	AnnotationKeyHumanDeployDependency   = "werf.io/deploy-dependency-<name>"
	AnnotationKeyPatternDeployDependency = regexp.MustCompile(`^werf.io/deploy-dependency-(?P<id>.+)$`)

	AnnotationKeyHumanDependency   = "<name>.dependency.werf.io"
	AnnotationKeyPatternDependency = regexp.MustCompile(`^(?P<id>.+).dependency.werf.io$`)

	AnnotationKeyHumanExternalDependency   = "<name>.external-dependency.werf.io"
	AnnotationKeyPatternExternalDependency = regexp.MustCompile(`^(?P<id>.+).external-dependency.werf.io$`)

	AnnotationKeyHumanLegacyExternalDependencyResource   = "<name>.external-dependency.werf.io/resource"
	AnnotationKeyPatternLegacyExternalDependencyResource = regexp.MustCompile(`^(?P<id>.+).external-dependency.werf.io/resource$`)

	AnnotationKeyHumanLegacyExternalDependencyNamespace   = "<name>.external-dependency.werf.io/namespace"
	AnnotationKeyPatternLegacyExternalDependencyNamespace = regexp.MustCompile(`^(?P<id>.+).external-dependency.werf.io/namespace$`)

	AnnotationKeyHumanSensitive   = "werf.io/sensitive"
	AnnotationKeyPatternSensitive = regexp.MustCompile(`^werf.io/sensitive$`)

	AnnotationKeyHumanSensitivePaths   = "werf.io/sensitive-paths"
	AnnotationKeyPatternSensitivePaths = regexp.MustCompile(`^werf.io/sensitive-paths$`)

	AnnotationKeyHumanDeployOn   = "werf.io/deploy-on"
	AnnotationKeyPatternDeployOn = regexp.MustCompile(`^werf.io/deploy-on$`)

	AnnotationKeyHumanOwnership   = "werf.io/ownership"
	AnnotationKeyPatternOwnership = regexp.MustCompile(`^werf.io/ownership$`)

	AnnotationKeyHumanDeletePropagation   = "werf.io/delete-propagation"
	AnnotationKeyPatternDeletePropagation = regexp.MustCompile(`^werf.io/delete-propagation$`)
)
var DefaultRegistryCredentialsPath = filepath.Join(homedir.Get(), ".docker", config.ConfigFileName)
var OrderedStoreAs = []StoreAs{StoreAsNone, StoreAsHook, StoreAsRegular}
var SprigFuncs = sprig.TxtFuncMap()
var StagesOrdered = []Stage{
	StageInit,
	StagePrePreUninstall,
	StagePrePreInstall,
	StagePreInstall,
	StagePreUninstall,
	StageInstall,
	StageUninstall,
	StagePostInstall,
	StagePostUninstall,
	StagePostPostInstall,
	StagePostPostUninstall,
	StageFinal,
}

FUNCTIONS

func StagesSortHandler(stage1, stage2 Stage) bool

TYPES

type ChartRepoConnectionOptions struct {
	ChartRepoBasicAuthPassword string
	ChartRepoBasicAuthUsername string
	ChartRepoCAPath string
	ChartRepoCertPath string
	ChartRepoInsecure bool
	ChartRepoKeyPath string
	ChartRepoPassCreds bool
	ChartRepoRequestTimeout time.Duration
	ChartRepoSkipTLSVerify bool
	ChartRepoURL string
}

func (opts *ChartRepoConnectionOptions) ApplyDefaults()

type DeletePolicy string
    Configures resource deletions during deployment of this resource.

const (
	DeletePolicySucceeded DeletePolicy = "succeeded"
	DeletePolicyFailed DeletePolicy = "failed"
	DeletePolicyBeforeCreation DeletePolicy = "before-creation"
	DeletePolicyBeforeCreationIfImmutable DeletePolicy = "before-creation-if-immutable"
)
type DeployType string
    Type of the current operation.

const (
	DeployTypeInitial DeployType = "Initial"
	DeployTypeInstall DeployType = "Install"
	DeployTypeUpgrade DeployType = "Upgrade"
	DeployTypeRollback DeployType = "Rollback"
	DeployTypeUninstall DeployType = "Uninstall"
)
type KubeConnectionOptions struct {
	KubeAPIServerAddress string
	KubeAuthProviderConfig map[string]string
	KubeAuthProviderName string
	KubeBasicAuthPassword string
	KubeBasicAuthUsername string
	KubeBearerTokenData string
	KubeBearerTokenPath string
	KubeBurstLimit int
	KubeConfigBase64 string
	KubeConfigPaths []string
	KubeContextCluster string
	KubeContextCurrent string
	KubeContextUser string
	KubeImpersonateGroups []string
	KubeImpersonateUID string
	KubeImpersonateUser string
	KubeProxyURL string
	KubeQPSLimit int
	KubeRequestTimeout time.Duration
	KubeSkipTLSVerify bool
	KubeTLSCAData string
	KubeTLSCAPath string
	KubeTLSClientCertData string
	KubeTLSClientCertPath string
	KubeTLSClientKeyData string
	KubeTLSClientKeyPath string
	KubeTLSServerName string
}

func (opts *KubeConnectionOptions) ApplyDefaults(homeDir string)

type On string
    On which action type the resource should be rendered for the deployment.

const (
	InstallOnInstall On = "install"
	InstallOnUpgrade On = "upgrade"
	InstallOnRollback On = "rollback"
	InstallOnDelete On = "delete"
	InstallOnTest On = "test"
)
type Ownership string
    Resource ownership.

const (
	OwnershipAnyone Ownership = "anyone"
	OwnershipRelease Ownership = "release"
)
type ResourceState string
    The state of the resource in the cluster.

const (
	ResourceStateAbsent  ResourceState = "absent"
	ResourceStatePresent ResourceState = "present"
	ResourceStateReady   ResourceState = "ready"
)
type SecretValuesOptions struct {
	DefaultSecretValuesDisable bool
	SecretKey string
	SecretKeyIgnore bool
	SecretValuesFiles []string
	SecretWorkDir string
}

func (opts *SecretValuesOptions) ApplyDefaults(currentDir string)

type Stage string
    A sequential stage of the plan.

const (
	StageInit              Stage = "init"                // create pending release
	StagePrePreUninstall   Stage = "pre-pre-uninstall"   // uninstall previous release resources
	StagePrePreInstall     Stage = "pre-pre-install"     // install crd
	StagePreInstall        Stage = "pre-install"         // install pre-hooks
	StagePreUninstall      Stage = "pre-uninstall"       // cleanup pre-hooks
	StageInstall           Stage = "install"             // install resources
	StageUninstall         Stage = "uninstall"           // cleanup resources
	StagePostInstall       Stage = "post-install"        // install post-hooks
	StagePostUninstall     Stage = "post-uninstall"      // cleanup post-hooks
	StagePostPostInstall   Stage = "post-post-install"   // install webhook
	StagePostPostUninstall Stage = "post-post-uninstall" // uninstall crd, webhook
	StageFinal             Stage = "final"               // succeed pending release, supersede previous release
)
func SubStageWeighted(stage Stage, weight int) Stage

type StoreAs string
    How the resource should be stored in the Helm release.

const (
	StoreAsNone    StoreAs = "none"
	StoreAsHook    StoreAs = "hook"
	StoreAsRegular StoreAs = "regular"
)
type TrackingOptions struct {
	LegacyHelmCompatibleTracking bool
	NoFinalTracking bool
	NoPodLogs bool
	NoProgressTablePrint bool
	ProgressTablePrintInterval time.Duration
	TrackCreationTimeout time.Duration
	TrackDeletionTimeout time.Duration
	TrackReadinessTimeout time.Duration
}

func (opts *TrackingOptions) ApplyDefaults()

type ValuesOptions struct {
	DefaultValuesDisable bool
	RuntimeSetJSON []string
	ValuesFiles []string
	ValuesSet []string
	ValuesSetFile []string
	ValuesSetJSON []string
	ValuesSetLiteral []string
	ValuesSetString []string
}

func (opts *ValuesOptions) ApplyDefaults()

~~~~

### Package

~~~~
package featgate // import "github.com/werf/nelm/pkg/featgate"


VARIABLES

var (
	FeatGateEnvVarsPrefix = caps.ToScreamingSnake(common.Brand) + "_FEAT_"
	FeatGates = []*FeatGate{}

	FeatGateRemoteCharts = NewFeatGate(
		"remote-charts",
		`Allow not only local, but also remote charts as an argument to cli commands. Also adds the "--chart-version" option`,
	)

	FeatGateNativeReleaseList = NewFeatGate(
		"native-release-list",
		`Use the native "release list" command instead of "helm list" exposed as "release list"`,
	)

	FeatGatePeriodicStackTraces = NewFeatGate(
		"periodic-stack-traces",
		`Print stack traces periodically to help with debugging deadlocks and other issues`,
	)

	FeatGateNativeReleaseUninstall = NewFeatGate(
		"native-release-uninstall",
		`Use the new "release uninstall" command implementation (not fully backwards compatible)`,
	)

	FeatGateFieldSensitive = NewFeatGate(
		"field-sensitive",
		`Enable JSONPath-based selective sensitive field redaction`,
	)

	FeatGatePreviewV2 = NewFeatGate(
		"preview-v2",
		`Activate all feature gates that will be enabled by default in Nelm v2`,
	)

	FeatGateCleanNullFields = NewFeatGate(
		"clean-null-fields",
		`Enable cleaning of null fields from resource manifests for better Helm chart compatibility`,
	)

	FeatGateMoreDetailedExitCodeForPlan = NewFeatGate(
		"more-detailed-exit-code-for-plan",
		`Make the "plan" command with the flag "--exit-code" return an exit code 3 instead of 2 when no resource changes, but still must install the release`,
	)
)

TYPES

type FeatGate struct {
	Name string
	Help string

}
    A feature gate, which enabled/disables a specific feature. Can be toggled
    via an env var or programmatically.

func NewFeatGate(name, help string) *FeatGate

func (g *FeatGate) Default() bool

func (g *FeatGate) Disable()

func (g *FeatGate) Enable()

func (g *FeatGate) Enabled() bool

func (g *FeatGate) EnvVarName() string

~~~~

### Package

~~~~
package secret // import "github.com/werf/nelm/pkg/legacy/secret"


FUNCTIONS

func ExpectedFilePathOrPipeError() error
func InputFromInteractiveStdin(prompt string) ([]byte, error)
func InputFromStdin() ([]byte, error)
func RotateSecretKey(
	ctx context.Context,
	helmChartDir string,
	secretWorkingDir string,
	secretValuesPaths ...string,
) error
func SaveGeneratedData(filePath string, data []byte) error
func SecretEdit(
	ctx context.Context,
	m *secrets_manager.SecretsManager,
	workingDir, tempDir, filePath string,
	values bool,
) error
func SecretFileDecrypt(
	ctx context.Context,
	m *secrets_manager.SecretsManager,
	workingDir, filePath, outputFilePath string,
) error
func SecretFileEncrypt(
	ctx context.Context,
	m *secrets_manager.SecretsManager,
	workingDir, filePath, outputFilePath string,
) error
func SecretValuesDecrypt(
	ctx context.Context,
	m *secrets_manager.SecretsManager,
	workingDir, filePath, outputFilePath string,
) error
func SecretValuesEncrypt(
	ctx context.Context,
	m *secrets_manager.SecretsManager,
	workingDir, filePath, outputFilePath string,
) error

TYPES

type GenerateOptions struct {
	FilePath       string
	OutputFilePath string
	Values         bool
}

~~~~

### Package

~~~~
package log // import "github.com/werf/nelm/pkg/log"


CONSTANTS

const (
	LogColorModeAuto = "auto"
	LogColorModeOff  = "off"
	LogColorModeOn   = "on"
)
const LogboekLoggerCtxKeyName = "logboek_logger"

VARIABLES

var Levels = []Level{SilentLevel, ErrorLevel, WarningLevel, InfoLevel, DebugLevel, TraceLevel}
var LogColorModes = []string{LogColorModeAuto, LogColorModeOff, LogColorModeOn}

FUNCTIONS

func SetupLogging(ctx context.Context, logLevel Level, opts SetupLoggingOptions) context.Context
    Sets up logging levels, colors, output formats, etc.


TYPES

type BlockOptions struct {
	BlockTitle string
}

type Level string

const (
	SilentLevel  Level = "silent"
	ErrorLevel   Level = "error"
	WarningLevel Level = "warning"
	InfoLevel    Level = "info"
	DebugLevel   Level = "debug"
	TraceLevel   Level = "trace"
)
type LogboekLogger struct {
}

func NewLogboekLogger() *LogboekLogger

func (l *LogboekLogger) AcceptLevel(ctx context.Context, lvl Level) bool

func (l *LogboekLogger) BlockContentWidth(ctx context.Context) int

func (l *LogboekLogger) Debug(ctx context.Context, format string, a ...interface{})

func (l *LogboekLogger) DebugPop(ctx context.Context, group string)

func (l *LogboekLogger) DebugPush(ctx context.Context, group, format string, a ...interface{})

func (l *LogboekLogger) Error(ctx context.Context, format string, a ...interface{})

func (l *LogboekLogger) ErrorPop(ctx context.Context, group string)

func (l *LogboekLogger) ErrorPush(ctx context.Context, group, format string, a ...interface{})

func (l *LogboekLogger) Info(ctx context.Context, format string, a ...interface{})

func (l *LogboekLogger) InfoBlock(ctx context.Context, opts BlockOptions, fn func())

func (l *LogboekLogger) InfoBlockErr(ctx context.Context, opts BlockOptions, fn func() error) error

func (l *LogboekLogger) InfoPop(ctx context.Context, group string)

func (l *LogboekLogger) InfoPush(ctx context.Context, group, format string, a ...interface{})

func (l *LogboekLogger) Level(ctx context.Context) Level

func (l *LogboekLogger) SetLevel(ctx context.Context, lvl Level)

func (l *LogboekLogger) Trace(ctx context.Context, format string, a ...interface{})

func (l *LogboekLogger) TracePop(ctx context.Context, group string)

func (l *LogboekLogger) TracePush(ctx context.Context, group, format string, a ...interface{})

func (l *LogboekLogger) TraceStruct(ctx context.Context, obj interface{}, format string, a ...interface{})

func (l *LogboekLogger) Warn(ctx context.Context, format string, a ...interface{})

func (l *LogboekLogger) WarnPop(ctx context.Context, group string)

func (l *LogboekLogger) WarnPush(ctx context.Context, group, format string, a ...interface{})

type Logger interface {
	Trace(ctx context.Context, format string, a ...interface{})
	TraceStruct(ctx context.Context, obj interface{}, format string, a ...interface{})
	TracePush(ctx context.Context, group, format string, a ...interface{})
	TracePop(ctx context.Context, group string)
	Debug(ctx context.Context, format string, a ...interface{})
	DebugPush(ctx context.Context, group, format string, a ...interface{})
	DebugPop(ctx context.Context, group string)
	Info(ctx context.Context, format string, a ...interface{})
	InfoPush(ctx context.Context, group, format string, a ...interface{})
	InfoPop(ctx context.Context, group string)
	Warn(ctx context.Context, format string, a ...interface{})
	WarnPush(ctx context.Context, group, format string, a ...interface{})
	WarnPop(ctx context.Context, group string)
	Error(ctx context.Context, format string, a ...interface{})
	ErrorPush(ctx context.Context, group, format string, a ...interface{})
	ErrorPop(ctx context.Context, group string)
	InfoBlock(ctx context.Context, opts BlockOptions, fn func())
	InfoBlockErr(ctx context.Context, opts BlockOptions, fn func() error) error
	BlockContentWidth(ctx context.Context) int
	SetLevel(ctx context.Context, lvl Level)
	Level(ctx context.Context) Level
	AcceptLevel(ctx context.Context, lvl Level) bool
}

var Default Logger = NewLogboekLogger()
type SetupLoggingOptions struct {
	ColorMode      string
	LogIsParseable bool
}

~~~~

