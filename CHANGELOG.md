# Changelog

## [1.15.0](https://github.com/werf/nelm/compare/v1.14.1...v1.15.0) (2025-10-17)


### Features

* option `--set-runtime-json` ([948a502](https://github.com/werf/nelm/commit/948a502971ec9468a041a1725cfc666019769b70))


### Bug Fixes

* error `too many arguments in call to chartutil.ToRenderValues` ([cd349f3](https://github.com/werf/nelm/commit/cd349f36e00c23e51eb30e6182c31617156ed7ab))
* invalid `helm.sh/hook` causes panic in `render` ([802708f](https://github.com/werf/nelm/commit/802708f0fab524e2aed4461752e9df979833ac4d))

### [1.14.1](https://www.github.com/werf/nelm/compare/v1.14.0...v1.14.1) (2025-10-16)


### Bug Fixes

* broken diffs with `object:` in `plan` ([97fceea](https://www.github.com/werf/nelm/commit/97fceeaba5e8bda53d87d40e80bb8d838ba74bc3))
* improve diffs in `plan` output ([90fd841](https://www.github.com/werf/nelm/commit/90fd841a5e4b0ce23812cd7120499ef43038d8d5))
* improve output of experimental `release list` ([902b6d9](https://www.github.com/werf/nelm/commit/902b6d9e1dd8cc07dd72e855d89a550a9e5b4078))

## [1.14.0](https://www.github.com/werf/nelm/compare/v1.13.2...v1.14.0) (2025-10-16)


### Features

* ability to use werf.io/delete-policy: before-creation-if-immutable annotation in case of field is immutable error ([e6b648f](https://www.github.com/werf/nelm/commit/e6b648f4d965b371b30e7cc8c9a672f5f591493c))
* option `--no-final-tracking` ([439cd63](https://www.github.com/werf/nelm/commit/439cd63bc2c036476b3ec46a8c516ea9e1a8c411))
* option `--no-remove-manual-changes` from the cluster resources ([ce489f3](https://www.github.com/werf/nelm/commit/ce489f3332a7918572094592dde0a39654039cf5))
* recreate non-hook Jobs on `field is immutable` by default ([4b2521f](https://www.github.com/werf/nelm/commit/4b2521f22321be9ce89568748973fb98bf489430))

### [1.13.2](https://www.github.com/werf/nelm/compare/v1.13.1...v1.13.2) (2025-10-13)


### Bug Fixes

* panic in `release plan install` ([f682b8d](https://www.github.com/werf/nelm/commit/f682b8da6f97812b6b24d8d2636b6a6e4a181b6b))

### [1.13.1](https://www.github.com/werf/nelm/compare/v1.13.0...v1.13.1) (2025-10-09)


### Bug Fixes

* `plan` panic and json diffs instead of yaml ([737cd51](https://www.github.com/werf/nelm/commit/737cd518132070c05f088cb314ae9645e48fa3fb))
* git-go security vulnerability ([80ae9d3](https://www.github.com/werf/nelm/commit/80ae9d3c2d7e92c473fd5db8bf44d2f9789a6b42))

## [1.13.0](https://www.github.com/werf/nelm/compare/v1.12.2...v1.13.0) (2025-10-08)


### Features

* `release plan install` options `--diff-context-lines`, `--show-insignificant-diffs`, `--show-sensitive-diffs`, `--show-verbose-crd-diffs`, `--show-verbose-diffs` ([fc1dd5d](https://www.github.com/werf/nelm/commit/fc1dd5d4aa1d4c310ae930121ccf77267ab130a6))
* major refactor, `werf.io/delete-policy`, `werf.io/ownership`, `werf.io/deploy-on` and more ([07d21f7](https://www.github.com/werf/nelm/commit/07d21f7e5e54653919bd46d429ead86272770540))

### [1.12.2](https://www.github.com/werf/nelm/compare/v1.12.1...v1.12.2) (2025-08-15)


### Bug Fixes

* release namespace deletes after stopping being part of a release ([2bba22b](https://www.github.com/werf/nelm/commit/2bba22bf08f5e14f8b5851e23ab16425698d17d6))

### [1.12.1](https://www.github.com/werf/nelm/compare/v1.12.0...v1.12.1) (2025-08-14)


### Bug Fixes

* error `"werf.io/show-logs-only-for-containers", expected integer value` ([209bd1c](https://www.github.com/werf/nelm/commit/209bd1c5ae9201426cb047435cec2fdaf5cfae48))

## [1.12.0](https://www.github.com/werf/nelm/compare/v1.11.0...v1.12.0) (2025-08-13)


### Features

* display logs only from 1 replica by default (configured with annotation `werf.io/show-logs-only-for-number-of-replicas`) ([47072bf](https://www.github.com/werf/nelm/commit/47072bf102d4a366f3ee00bed07182296c4cece8))

## [1.11.0](https://www.github.com/werf/nelm/compare/v1.10.0...v1.11.0) (2025-08-08)


### Features

* greatly decrease Kubernetes apiserver load ([7afe7ad](https://www.github.com/werf/nelm/commit/7afe7ad1b5a9ec0b3c5301f5d51b82d5d51f947e))


### Bug Fixes

* panic "rules file not valid" ([075b8e0](https://www.github.com/werf/nelm/commit/075b8e0f658ce708d140c4c07a6668b74bb6ec21))
* panic "validate rules file" ([6bb4e3b](https://www.github.com/werf/nelm/commit/6bb4e3b43b8582a08584b588cd0d7babf88819c8))

## [1.10.0](https://www.github.com/werf/nelm/compare/v1.9.0...v1.10.0) (2025-08-01)


### Features

* add `werf.io/log-regex-skip` annotation ([3da6610](https://www.github.com/werf/nelm/commit/3da6610d05c5e2baaf14756d7835f2ed13693ccf))
* **deploy, tracking:** update kubedog to track prometheus resources ([5e7dc4a](https://www.github.com/werf/nelm/commit/5e7dc4adbffc53058871878dff797c1033fc692f))


### Bug Fixes

* extra and some secret values wrongly propagated to subcharts ([8ffa419](https://www.github.com/werf/nelm/commit/8ffa419578757452d5d78e5064c7a3469af78fde))

## [1.9.0](https://www.github.com/werf/nelm/compare/v1.8.0...v1.9.0) (2025-07-29)


### Features

* **action/release:** ignore pod's logs using NoLogs option ([77e7fb1](https://www.github.com/werf/nelm/commit/77e7fb1dcd5485085cad8dd147e8691da4ba502c))
* **cli:** release install, release rollback and release uninstall should have --no-pod-logs option ([#386](https://www.github.com/werf/nelm/issues/386)) ([b186537](https://www.github.com/werf/nelm/commit/b1865375b4140948f73b7acb2bc2cbc760cbaaeb))


### Bug Fixes

* non-successful release revisions since last successful never cleaned up ([9dc59d8](https://www.github.com/werf/nelm/commit/9dc59d82d2ddb81c90fda24b9ff660a3eafe2816))

## [1.8.0](https://www.github.com/werf/nelm/compare/v1.7.2...v1.8.0) (2025-07-11)


### Features

* werf.io/sensitive-paths annotation and WERF_FEAT_FIELD_SENSITIVE featgate ([#364](https://www.github.com/werf/nelm/issues/364)) ([e3f9798](https://www.github.com/werf/nelm/commit/e3f97984dbb8dc3a13e186284f49b72efc9943f4))


### Bug Fixes

* leaking goroutines during tracking ([1c1be03](https://www.github.com/werf/nelm/commit/1c1be031e43311e015be06fc4ed07c46ec785fe2))
* logs from libraries still showed by default ([c6b3928](https://www.github.com/werf/nelm/commit/c6b39287b0c132b324b7d9ff26b43d769dc6bce9))

### [1.7.2](https://www.github.com/werf/nelm/compare/v1.7.1...v1.7.2) (2025-07-10)


### Bug Fixes

* no logs displayed ([39a92da](https://www.github.com/werf/nelm/commit/39a92da4f64195e02933b6f4fec105ae91d7409d))

### [1.7.1](https://www.github.com/werf/nelm/compare/v1.7.0...v1.7.1) (2025-07-10)


### Bug Fixes

* hide libraries logs unless log level is debug ([69dc532](https://www.github.com/werf/nelm/commit/69dc5325dda560e8bf57c953297faee30a4dde85))
* improve context cancellation handling ([b2b5b93](https://www.github.com/werf/nelm/commit/b2b5b936e097ecc9dae68366ce25e8ff165f53f9))

## [1.7.0](https://www.github.com/werf/nelm/compare/v1.6.0...v1.7.0) (2025-06-18)


### Features

* `--force-adoption` to disable release annotations checks ([b1dd851](https://www.github.com/werf/nelm/commit/b1dd8519a41ae7862880d752b01ef73dc2284e60))
* expose release labels in `release get` ([471c999](https://www.github.com/werf/nelm/commit/471c999a7a47fc738366f0143fa5d824e874a4cf))

## [1.6.0](https://www.github.com/werf/nelm/compare/v1.5.0...v1.6.0) (2025-06-09)


### Features

* include_debug/tpl_debug/printf_debug/dump_debug and detailed template errors ([ee22873](https://www.github.com/werf/nelm/commit/ee228734f5cc48341d3659bc3b32bfc3f6bcea72))

## [1.5.0](https://www.github.com/werf/nelm/compare/v1.4.1...v1.5.0) (2025-06-04)


### Features

* `NELM_FEAT_PREVIEW_V2` feature gate ([f7ad0ab](https://www.github.com/werf/nelm/commit/f7ad0abc92c5b1462e56ddf13a037752c755278e))
* native `release list` command ([ff9e1f0](https://www.github.com/werf/nelm/commit/ff9e1f089ea9099cd3a874592a69595f988167f4))
* native `release uninstall` command ([4d85484](https://www.github.com/werf/nelm/commit/4d854849446a233c4a08515570313a8777bf6c1b))


### Bug Fixes

* show stack traces with `NELM_FEAT_PERIODIC_STACK_TRACES=true` ([20310db](https://www.github.com/werf/nelm/commit/20310db2290a42d229a807c17e1352d23911cfeb))

### [1.4.1](https://www.github.com/werf/nelm/compare/v1.4.0...v1.4.1) (2025-05-23)


### Bug Fixes

* improve values handling; add more trace logs ([358855f](https://www.github.com/werf/nelm/commit/358855febc657cb326076efae9ac225698b7a253))

## [1.4.0](https://www.github.com/werf/nelm/compare/v1.3.0...v1.4.0) (2025-05-14)


### Features

* `--no-install-crds` for `release install/plan` ([efc22bc](https://www.github.com/werf/nelm/commit/efc22bca73d07af229c351ca74f3e29ebb571f44))
* `--print-values` option for `release get` ([f41f615](https://www.github.com/werf/nelm/commit/f41f615b3917da22d333d6d588b7b7adf2f9505e))
* `--release-labels` option for `release install` ([9b20bc0](https://www.github.com/werf/nelm/commit/9b20bc0c63e8651ce3cd728fcf9d8d471da12652))
* `--timeout` option for `release install/rollback/uninstall/plan` ([d563296](https://www.github.com/werf/nelm/commit/d563296ac4866f3d9ec0030308acb7f9ef20211a))


### Bug Fixes

* disallow unknown NELM_FEAT_.* env vars ([7e25a16](https://www.github.com/werf/nelm/commit/7e25a16f5f0c40d94308c77d18cad5cee31d5194))

## [1.3.0](https://www.github.com/werf/nelm/compare/v1.2.2...v1.3.0) (2025-05-07)


### Features

* allow specifying remote charts in cli commands ([b21329c](https://www.github.com/werf/nelm/commit/b21329cb4cc745747b1f1979c27f2c60d35c526a))

### [1.2.2](https://www.github.com/werf/nelm/compare/v1.2.1...v1.2.2) (2025-04-30)


### Bug Fixes

* improve log color detection ([9728f02](https://www.github.com/werf/nelm/commit/9728f021b369aa46bb1fbe70dff41461a5c003a5))

### [1.2.1](https://www.github.com/werf/nelm/compare/v1.2.0...v1.2.1) (2025-04-25)


### Bug Fixes

* `release plan` shows insignificant changes for no reason ([66d500b](https://www.github.com/werf/nelm/commit/66d500b65bc4042456589a09c6ab777529a92424))

## [1.2.0](https://www.github.com/werf/nelm/compare/v1.1.5...v1.2.0) (2025-04-23)


### Features

* enable colors by default in some popular CI systems ([d3aa7b8](https://www.github.com/werf/nelm/commit/d3aa7b82f911418be846982e4e05b71206eae308))


### Bug Fixes

* Buildah reexec sets wrong HOME dir ([4eb656e](https://www.github.com/werf/nelm/commit/4eb656e82a8ba38b4228cf7a80449f297211255b))
* default kubeconfig not used on empty string kubeconfigpath action option ([ffc2132](https://www.github.com/werf/nelm/commit/ffc21329922511721326ce0b3201a56c4a3201d3))

### [1.1.5](https://www.github.com/werf/nelm/compare/v1.1.4...v1.1.5) (2025-04-14)


### Bug Fixes

* chart dir path and revision options/arguments to some commands ignored ([591b60f](https://www.github.com/werf/nelm/commit/591b60f1b96dac65d58cfa960e1ab3c1a04636f2))

### [1.1.4](https://www.github.com/werf/nelm/compare/v1.1.3...v1.1.4) (2025-04-11)


### Bug Fixes

* allow `werf.io/sensitive: false` for Secrets ([53665fd](https://www.github.com/werf/nelm/commit/53665fd3b9a218dc2e67fb69df4ffb0803f5022e))
* possible panic in tracking flux canary resource ([c6861bf](https://www.github.com/werf/nelm/commit/c6861bf5391b60d392cb648a1a482c23b489e72e))

### [1.1.3](https://www.github.com/werf/nelm/compare/v1.1.2...v1.1.3) (2025-04-11)


### Bug Fixes

* `nelm version` shows `0.0.0` ([5d79185](https://www.github.com/werf/nelm/commit/5d79185823cce4dfa94dbb72137d63542b648e11))

### [1.1.2](https://www.github.com/werf/nelm/compare/v1.1.1...v1.1.2) (2025-04-10)


### Bug Fixes

* error if `$KUBECONFIG` has multiple files; major rework of K8s client setup ([5291873](https://www.github.com/werf/nelm/commit/5291873189c30d1dbfcde22a16cba968929a0ebe))

### [1.1.1](https://www.github.com/werf/nelm/compare/v1.1.0...v1.1.1) (2025-03-31)


### Bug Fixes

* make `info` log level less verbose ([0de72ff](https://www.github.com/werf/nelm/commit/0de72ffd693a8d9d129f149599f26b09e5de3b49))
* wrong `nelm version` output ([176ba60](https://www.github.com/werf/nelm/commit/176ba6056427d4cfac0ce356899dbd0feed6b949))

## [1.1.0](https://www.github.com/werf/nelm/compare/v1.0.0...v1.1.0) (2025-03-24)


### Features

* `--release-info-annotations` for `release install`, exposed in `release get` ([e563a61](https://www.github.com/werf/nelm/commit/e563a61ee1bc9aa42e0c932d464375588b80774d))


### Bug Fixes

* calling invalid cli commands sometimes returns 0 exit code ([571a87c](https://www.github.com/werf/nelm/commit/571a87c2f6afd50aca5e046b50b6395fa4e73c27))

## 1.0.0 (2025-03-20)


### Features

* `SecretKeyRotate` action ([c01298a](https://www.github.com/werf/nelm/commit/c01298ae45f1277739d94d1913b4b8a8e34e5896))
* add `chart dependency update` command ([70ab11c](https://www.github.com/werf/nelm/commit/70ab11c8ddb74e5323c725928921c9a0b336c757))
* add `chart download/upload/archive commands` ([e783327](https://www.github.com/werf/nelm/commit/e7833270e94d727ef5e10a755296501222edc8ae))
* add `chart lint` command and `Lint` action ([5672c82](https://www.github.com/werf/nelm/commit/5672c826fdd9108f85468b0352a0612027e50bcf))
* add `chart secret file encrypt/decrypt/edit` commands ([65d7127](https://www.github.com/werf/nelm/commit/65d7127282d00670c82103444faf4c6556e2f4b0))
* add `chart secret key create/rotate` commands ([5979513](https://www.github.com/werf/nelm/commit/59795138959fac111fe2c40dd64e966c7ea25657))
* add `chart secret values-file encrypt/decrypt/edit commands`, move some secrets code from werf to pkg/secret ([39777c6](https://www.github.com/werf/nelm/commit/39777c6c3c51f0c8914caab9ac025bd27f479918))
* add `release get` command and `Get` action ([bb496ee](https://www.github.com/werf/nelm/commit/bb496eee781f07b911188c32a4643f011d84b081))
* add `release history` command ([df782af](https://www.github.com/werf/nelm/commit/df782af1c60cf35bf91182e6c0d59a261146c977))
* add `release list` command ([1474b81](https://www.github.com/werf/nelm/commit/1474b81e7feb448d0ddffc576c4deb2ab500664d))
* add `release rollback` command ([4af458b](https://www.github.com/werf/nelm/commit/4af458b9ec4393c8069bc3e6ce6b9da6417bb1d1))
* add `repo add/remove/update/login/logout` commands ([e917b65](https://www.github.com/werf/nelm/commit/e917b655a96c1080031cc7b31eac3202dc31fb52))
* add `Rollback` action ([eb4ee7f](https://www.github.com/werf/nelm/commit/eb4ee7fe9714a586230ff57f0aba10e1e7f49d48))
* add `SecretFileDecrypt/Encrypt/Edit` actions ([a25566a](https://www.github.com/werf/nelm/commit/a25566ad0323593248a628ecb240665ae149e389))
* add `SecretKey` option to actions ([10edcbd](https://www.github.com/werf/nelm/commit/10edcbd1e2b3fe535eef87e5ead0a330f9585018))
* add `SecretKeyCreate` action ([f75fd09](https://www.github.com/werf/nelm/commit/f75fd09810624f6bd275ed17db809a73fe3c2b9a))
* add `SecretValuesDecrypt` action ([94b17f8](https://www.github.com/werf/nelm/commit/94b17f8930b2233ccee9866fd5e93acbf6639196))
* add `SecretValuesFileEdit` action ([500abad](https://www.github.com/werf/nelm/commit/500abad987924f77434d4fdffe79be987b745d14))
* add `SecretValuesFileEncrypt` action ([42bc967](https://www.github.com/werf/nelm/commit/42bc96776b5fbaa4804957888ad72ba8e0916c4e))
* add `version` command ([cf9a441](https://www.github.com/werf/nelm/commit/cf9a44188c7a9cf343fa00f115868a36fa8679d1))
* add `werf.io/sensitive` annotation ([7840c7c](https://www.github.com/werf/nelm/commit/7840c7c7ec6301165c7a395aa844893804f5708e))
* add Dockerfile for nelm CLI build and deployment ([05f9e3b](https://www.github.com/werf/nelm/commit/05f9e3b6b73cafda623e425750bdfc21e6211f9d))
* add first original `chart dependency update` Helm command ([47f774b](https://www.github.com/werf/nelm/commit/47f774b62eaa980b6ae2f1801c80da77c0cf57ba))
* add flag description wrapping in --help ([34ccf2f](https://www.github.com/werf/nelm/commit/34ccf2fbad64c5e994d8abed1c850930a3c8b661))
* add more kube auth options to actions ([b6e439d](https://www.github.com/werf/nelm/commit/b6e439d8945d50196a491ccf6e1127cb9facc3f3))
* add optional Chart.yaml fallbacks for name/apiVersion/version/appVersion if missing ([a6ce234](https://www.github.com/werf/nelm/commit/a6ce234fd2b225ef248039303919abbe561df829))
* add Plan() action ([a86117f](https://www.github.com/werf/nelm/commit/a86117ff52d33ed0db6cb2af9842a9e502831d75))
* add Printer for new Dynamic Tracker ([a4337b0](https://www.github.com/werf/nelm/commit/a4337b05047b9ae21f2ff6e61c636b70e4cf52da))
* add public Deploy() action ([5176060](https://www.github.com/werf/nelm/commit/5176060adb8391dc08a35c370533f4ebd62b3ef6))
* add release differ ([96e8d25](https://www.github.com/werf/nelm/commit/96e8d253aceaea36b619b014c3aeba4120e7ceba))
* add Render() action ([79d212e](https://www.github.com/werf/nelm/commit/79d212e4c8c838eb114d99c8fc964d0519981cff))
* add some missing options to all cli commands ([ce826b9](https://www.github.com/werf/nelm/commit/ce826b97f9d29d1b67858e8400f195947f0a777f))
* add Uninstall() action ([77e9158](https://www.github.com/werf/nelm/commit/77e91583e28efa41642200d359f1855af629cade))
* allow processing resources without cluster access ([e300250](https://www.github.com/werf/nelm/commit/e300250fbfda29626e614b843abe5ad9ade1ece9))
* caching for kubernetes GET requests ([293ca66](https://www.github.com/werf/nelm/commit/293ca6662b2c3cd143d123527592413cc71b9efd))
* change cli commands layout and rename actions ([ca201e7](https://www.github.com/werf/nelm/commit/ca201e719754d01676d06c7fa7c260e10dd7de2e))
* change DeletePropagationPolicy from Auto to Foreground ([67f4598](https://www.github.com/werf/nelm/commit/67f459816b6be9bbe0ddb058001e9193a36ef666))
* change some cli options ([6e9cabe](https://www.github.com/werf/nelm/commit/6e9cabee8beb13fc1debbb3a51c9ad5a0367ef91))
* colorize `chart render` output ([17ab583](https://www.github.com/werf/nelm/commit/17ab583dbd2af1b22587aa4b325eb6a0df9498c7))
* colorize `release get` command output ([03c521d](https://www.github.com/werf/nelm/commit/03c521d532d4a9a21342a7015307e04b84e69fad))
* colorize `version` command output, refactor syntax highlighting ([3c87b6c](https://www.github.com/werf/nelm/commit/3c87b6c53a2ab230eda5fff2fe37ab94ff39ab11))
* complete rework of managed fields handling + minor fixes ([13d672d](https://www.github.com/werf/nelm/commit/13d672d35ed9accf68f3a364df4eeac763c01855))
* deploy plan changes printing, improved logging ([df454a0](https://www.github.com/werf/nelm/commit/df454a0960dc5ebc3ba546e9ed6de0f2aad7cfe4))
* **deps:** update Kubedog and Helm 3 ([4ac00b8](https://www.github.com/werf/nelm/commit/4ac00b85158ff0eecba958d28ef7bdcc2906cb3f))
* detailed exit code option for planner ([ab09d6c](https://www.github.com/werf/nelm/commit/ab09d6c4b4decf48772a4ab04cc4767411d549ef))
* enhance nelm CLI with new chart and release management commands ([01a670e](https://www.github.com/werf/nelm/commit/01a670e5a91f3f523d545291a4d1712694f6122e))
* expose deployable resources from resource processor ([9b36985](https://www.github.com/werf/nelm/commit/9b369857a2e214d4d45074dc2e8cd1141979cae8))
* ExternalSecret CR tracking works again ([247bf72](https://www.github.com/werf/nelm/commit/247bf7209a0a35f63f35669524627ded4b637bdb))
* group options in --help ([aa48edf](https://www.github.com/werf/nelm/commit/aa48edf113bb9085b5769b21833087caaa92642a))
* helm new deploy core v2 ([52b5648](https://www.github.com/werf/nelm/commit/52b56489ede3350013b3ae0b0a9554ec6e4616f2))
* hide sensitive Secret's diff ([7211e3c](https://www.github.com/werf/nelm/commit/7211e3cc98ef62c7c07dbfa6a29dd6dfad8e7598))
* implement nelm CLI with core commands and update README ([a553eeb](https://www.github.com/werf/nelm/commit/a553eeb3d8664937c7e79ade1af30334da891590))
* improve --help structure and formatting ([e903aa1](https://www.github.com/werf/nelm/commit/e903aa19610b198c1122fa920eaeb5d7c5794055))
* improve log coloring ([4351445](https://www.github.com/werf/nelm/commit/43514453dca18f45a631efebaff61639e34139c4))
* make env vars in --help more readable ([a54faa9](https://www.github.com/werf/nelm/commit/a54faa94fc3d9b1f13c54b076a812558c4c9bfaa))
* make Kube QPS and Burst limits configurable ([5b5426e](https://www.github.com/werf/nelm/commit/5b5426ee1f3ff707b254010abe3e6599ca400757))
* migrate to new kubedog dynamic tracker ([71c8ae0](https://www.github.com/werf/nelm/commit/71c8ae0fbdccc6dd56e97fcb6e18e22f5127aa7f))
* new `werf.io/deploy-dependency-<name>` annotation ([8d464f9](https://www.github.com/werf/nelm/commit/8d464f9dbbe640f2ac63a277dfea46a6c9368a1c))
* new experimental deploy engine ([d770963](https://www.github.com/werf/nelm/commit/d7709634e2c6477d3cf4b76743c99c68ebabb61f))
* print templated manifests when they are invalid with debug logging enabled ([1d995b5](https://www.github.com/werf/nelm/commit/1d995b5d58ce3a0d5a6b3c69fd7a65f41d262c2e))
* priority sorting, improved formatting and grouping for commands lists in `--help` ([a5732c8](https://www.github.com/werf/nelm/commit/a5732c8b7f544b9d686a3bafe1a3b06bd48f5fff))
* remade all cli flags for all commands ([ab77a18](https://www.github.com/werf/nelm/commit/ab77a182136392eaea08eca8b2ca80197e4801d9))
* remove excessive operations summary ([95e0c82](https://www.github.com/werf/nelm/commit/95e0c829d74a03d5b9b69b85126534866c92469f))
* remove mentions about Nelm being experimental and update README.md ([164d465](https://www.github.com/werf/nelm/commit/164d4655f5785900349d3165bff28b7baea3b532))
* rename `chart archive` command to `chart pack` ([8e2c3af](https://www.github.com/werf/nelm/commit/8e2c3af5fd6936cdea563b29b1b7b58c83487cb9))
* replace --debug with --log-level ([80b5d07](https://www.github.com/werf/nelm/commit/80b5d076fc9fe5b88b70d70bdc00a963adf0d7f0))
* rework CLI ([f7b518a](https://www.github.com/werf/nelm/commit/f7b518ab1a80b5dd573a138afc181d8451f18c82))
* set cli flags via env vars and refactor flag management ([2f428bb](https://www.github.com/werf/nelm/commit/2f428bb9058034e84adf42326aa38f983edc0541))
* skip removal of resources with missing/incorrect release annotations and labels ([5a39613](https://www.github.com/werf/nelm/commit/5a3961384b1f79153c588ab60c5bdd5ba5da7ea5))
* stable resource sorting everywhere ([ea2d973](https://www.github.com/werf/nelm/commit/ea2d973eb0a7483d0d21b7530b6a7d8e44bec599))
* update/sync all dependencies to github.com/werf/werf ([fa47525](https://www.github.com/werf/nelm/commit/fa47525cc8f3560dffc3dcfa17b011bcaada0f43))
* use improved custom syntax highlight theme for logs ([8c558a3](https://www.github.com/werf/nelm/commit/8c558a3135a6486f1d58d3ba5b74a032ebcca301))


### Bug Fixes

* "no matches for kind" when deploying CR for CRD from crds/ ([8df83c8](https://www.github.com/werf/nelm/commit/8df83c8c385741ec432bda896bcf040d400f8e2a))
* `--color-mode` doesn't work in `chart render` command ([348d3b3](https://www.github.com/werf/nelm/commit/348d3b3b4786f14cddeaf73a24a0e5416a58ce9d))
* `$HOME is not defined` and `function werf_secret_file not defined` ([f60246d](https://www.github.com/werf/nelm/commit/f60246d5ac1c2cac3882ebad2d78c780df9b1d3a))
* `SecretFileEdit` action `TempDirPath` option had no default ([b562db1](https://www.github.com/werf/nelm/commit/b562db11e712353421b9811230b94993ac072fcb))
* add `--log-level` to `version` command ([aa3ab9c](https://www.github.com/werf/nelm/commit/aa3ab9cdffcdaf7e36998722984fb1b4c15fef6a))
* add chart, log, errors packages ([a23b192](https://www.github.com/werf/nelm/commit/a23b19272fc32bfd8de5916bbc5be9c035017e4b))
* add global `actionLock` due to actions not be thread-safe ([12131b7](https://www.github.com/werf/nelm/commit/12131b781be14ea1ff5a8586bf065691f1a3129e))
* add History api ([d6045a8](https://www.github.com/werf/nelm/commit/d6045a8c8a667e6d3fb9beb93072a12b10a0b1e3))
* add info about options allowed values to `--help` ([96f92aa](https://www.github.com/werf/nelm/commit/96f92aa41defbd35ea49e3e67ea7654673daa935))
* add new "skipped" release status ([ce428fe](https://www.github.com/werf/nelm/commit/ce428fed20de736a72667938c19aebf004e0fcd0))
* allow empty helm.sh/hook-delete-policy and werf.io/delete-policy ([e3d1326](https://www.github.com/werf/nelm/commit/e3d132667d3369afea720f8de0f08d508daf3834))
* broken release manifests ([d002c72](https://www.github.com/werf/nelm/commit/d002c724f0bc3c4e583ce4ff341b53248956a9e4))
* cannot deep copy *annotation.AnnotationReplicasOnCreation ([77f475f](https://www.github.com/werf/nelm/commit/77f475f7c7207073da2919b4828745435c0dc1c0))
* change default log levels for all actions ([da4a0a2](https://www.github.com/werf/nelm/commit/da4a0a25efde9d2153fd41cc9525dbb589625320))
* change default log levels for all commands ([7f0e29a](https://www.github.com/werf/nelm/commit/7f0e29a106fe37d30a9ce144903b3e0a01b34903))
* change package name to github repo ([26f2d6a](https://www.github.com/werf/nelm/commit/26f2d6a0089d634764b336ecc6ffbd8865636a7f))
* change package name to github repo /2 ([314e6b9](https://www.github.com/werf/nelm/commit/314e6b925ac5fc2bd064063e955c786bfebbb092))
* change params to comma-separated in `werf.io/deploy-dependency-...` anno ([8fccfc8](https://www.github.com/werf/nelm/commit/8fccfc8f2787cdc6e843a72ac6572d8be43bb4f3))
* claim ownership of kubectl edit'ed fields ([321c57e](https://www.github.com/werf/nelm/commit/321c57ebb3df986f3bd06c362897a51d7158f504))
* CLI build fails with `undefined: releaseNameStub` ([67a1fd3](https://www.github.com/werf/nelm/commit/67a1fd3dbdc060f66c1ad3faf9bcb3b4f2dab3e6))
* colorize warnings and errors ([84cf471](https://www.github.com/werf/nelm/commit/84cf4718cd29202da22f0c3299aa6ddb0cf36d04))
* corrupted revision number in errors ([755a223](https://www.github.com/werf/nelm/commit/755a223dd61f5cd427359ed7e76e97e63510da50))
* cosmetics ([c7f020d](https://www.github.com/werf/nelm/commit/c7f020db9c51c8eb910abde9e8dc172982292cd1))
* cosmetics ([ce9fb45](https://www.github.com/werf/nelm/commit/ce9fb45c75a9f970620682833ec440201c9eb244))
* decouple pkgs from resource classes, refactor tracker and waiter ([ea441e1](https://www.github.com/werf/nelm/commit/ea441e17a7f824842faba2a0937738825408c318))
* dependencies could not find Apply vertices ([e3cf591](https://www.github.com/werf/nelm/commit/e3cf591249f5b61af75c5571893fadff2cd0554a))
* dependency between ClusterRoleBinding in non-release namespace and ClusterRole not detected ([30a4791](https://www.github.com/werf/nelm/commit/30a4791a4b0e493c52c61abaa6cbbbbb2bdd903d))
* deploy graph cycle if same external dependency on multiple resources ([befda66](https://www.github.com/werf/nelm/commit/befda663807b905c691350da6f6812f8c89f428d))
* deploy release builder to History api ([33e4685](https://www.github.com/werf/nelm/commit/33e4685d8e6c5588f3a203b3ba5aee47069a6609))
* deployable general resources not patching correctly ([62caec7](https://www.github.com/werf/nelm/commit/62caec77fae62e85c52c0824357b7f6e80125cb9))
* differ dry-run apply from real apply in logs ([d2c6df5](https://www.github.com/werf/nelm/commit/d2c6df52ce9fa84dffc01f8465903b5414c78387))
* disable rendering of subchart notes by default ([14c2fa9](https://www.github.com/werf/nelm/commit/14c2fa9d0e2549e1de7e03840d84360d7d54611b))
* divide by zero error if 0 deployable resources ([7a4126c](https://www.github.com/werf/nelm/commit/7a4126c80b3f52e89d7bc40ee978de8c5ddf8121))
* do not create internal dependency if pointing field is empty string ([17cd652](https://www.github.com/werf/nelm/commit/17cd652a42851a9f8ad5c1d9615935b09bbf1590))
* embeed Lists transformer into resource processor ([1800d8f](https://www.github.com/werf/nelm/commit/1800d8fe7e0f9f45a308c5d243e4c6db16e33395))
* engine v2 fixes ([72c09fd](https://www.github.com/werf/nelm/commit/72c09fdfa6fc32ef1fded41bba3e13968b3e4c82))
* env vars ignored for required cli options ([658820a](https://www.github.com/werf/nelm/commit/658820ad4d50129dd57b5e479babbff52296a268))
* error message 'annotation ... not found, must be set to ""' ([3cc6d4f](https://www.github.com/werf/nelm/commit/3cc6d4fe31c8477ddde1acc5dcb9bca7e16d2130))
* errors count in progress report not updating ([6a14413](https://www.github.com/werf/nelm/commit/6a14413815d6fde40dcb7e18c199b2609585d71c))
* External Secret CR readying when CR has no status ([b7cbcc8](https://www.github.com/werf/nelm/commit/b7cbcc831b5d7ce2b7b9494dd72b53304c1146ff))
* failure on first revision ([ac4845b](https://www.github.com/werf/nelm/commit/ac4845bb508c41c8688dea385aaf7730da15093d))
* field adoption didn't work if live resource up to date ([606e0df](https://www.github.com/werf/nelm/commit/606e0dfebe3dde191375a7cda0d19df210a9fd76))
* fix debug, fix noactivitytimeout for hooks ([c56bea9](https://www.github.com/werf/nelm/commit/c56bea9799eb72e213a11da86b89ce777703b934))
* fix links in release message ([3691943](https://www.github.com/werf/nelm/commit/3691943ff4489ee1f26a7c53e8205effa8da81c7))
* get deployable resources race condition, improve GET caching ([9f802d5](https://www.github.com/werf/nelm/commit/9f802d59a3e4bdccfb7f95e78d0a27693f8f48bc))
* get rid of kubedog module replace ([50a8e59](https://www.github.com/werf/nelm/commit/50a8e5975110390a7211eff3048460d894c3a800))
* hangs on panic during operation execution ([fea8447](https://www.github.com/werf/nelm/commit/fea84476ba7c1cce1c3cf4a53236085fa02c2a9c))
* helm debug logs printed on info level ([f738a22](https://www.github.com/werf/nelm/commit/f738a22187d575bba51934206c469fd5ad6780f0))
* helm hooks with multiple pre/post conditions always skipped ([0a4ad9b](https://www.github.com/werf/nelm/commit/0a4ad9bfe766c70e389871a8498e9f6aabefebac))
* hide CRDs create diffs from plan ([c3d9bcb](https://www.github.com/werf/nelm/commit/c3d9bcbc5a53e1430ace7da56c53b39f1c126e6a))
* hooks run twice ([1cece09](https://www.github.com/werf/nelm/commit/1cece097a4aabf3554a2612f0284f8020a0231a3))
* ignore error "Additional property werf is not allowed" ([9a8bbc0](https://www.github.com/werf/nelm/commit/9a8bbc0eb393b2d8454307ad06cc0dc71c16f3c5))
* ignore resource API Version in its ID ([5d786ea](https://www.github.com/werf/nelm/commit/5d786eab011cc0ec839fbca3682eb5885422d87a))
* improve logging ([b405dfc](https://www.github.com/werf/nelm/commit/b405dfcb0177166e9ed6611b8c2be51423b169ec))
* initial resource status should be "unknown" instead of "created" ([b1ce7af](https://www.github.com/werf/nelm/commit/b1ce7afcc889f39709ba39f265812fdc4c30d181))
* initialize Nelm as a package ([2a206e3](https://www.github.com/werf/nelm/commit/2a206e3b5da6189fcf7259510a1d7648b7422cff))
* invalid labels/annotations silently remove all user labels/annotations for the resource ([215b579](https://www.github.com/werf/nelm/commit/215b579edc81f7a17281bbecf24fd68affdd521c))
* Jobs not failing on errors ([85bdffb](https://www.github.com/werf/nelm/commit/85bdffbd25042522e25987eb94377c82d1848863))
* **locker:** replace panic with error and add details for lost lease case ([e553a09](https://www.github.com/werf/nelm/commit/e553a093de7bf6d40103152e8376a078570c89a0))
* minimize DeepCopying while processing resources ([72347ab](https://www.github.com/werf/nelm/commit/72347abf6ad25e3fbcb70c1561dde59b86205b29))
* minor cli and action api changes ([a351452](https://www.github.com/werf/nelm/commit/a35145244385d1e23a2913dd7c9f2b5ae929f8c9))
* missing filepath for hooks ([e5188c9](https://www.github.com/werf/nelm/commit/e5188c960ff984aa1f26799e1701f5063c23b745))
* multiple delete policies ignored ([82d624e](https://www.github.com/werf/nelm/commit/82d624e2313a382360a164468a5d75deac5ea5ae))
* new "resource" package, will replace "resource" and "annotation" ([7565cc3](https://www.github.com/werf/nelm/commit/7565cc3823b3c7e43285e2283809186c651b26cb))
* no need for field-manager to be configurable ([2addfc9](https://www.github.com/werf/nelm/commit/2addfc93c9ebd544a04e6aa4ab4cff667fe5c1f5))
* no need for field-manager to be configurable /2 ([27b50f0](https://www.github.com/werf/nelm/commit/27b50f0510047bb370822a5b828c10a5838a00e4))
* no need for field-manager to be configurable /3 ([eb9955f](https://www.github.com/werf/nelm/commit/eb9955f23cb619ff970efdbe98293106906e2998))
* no resources deleted in uninstall if pre-delete hook present ([5f1da12](https://www.github.com/werf/nelm/commit/5f1da12ae8b05a52646ee8a09da1ca527bcf1710))
* numerous bugs in new flag handling ([cf8884e](https://www.github.com/werf/nelm/commit/cf8884e16e0e0580ea9de0f7c1a700f5dc92f864))
* occasional panics on logging ([a36c897](https://www.github.com/werf/nelm/commit/a36c89719aaaa1545348a3b9f42759f9993d765d))
* panic connecting internal dependencies ([4818eaf](https://www.github.com/werf/nelm/commit/4818eaf95d2772523b3873e615edcecde3969917))
* panic if replicas null ([303f19a](https://www.github.com/werf/nelm/commit/303f19a133005d837a54745853b7e21e1b663614))
* panic in `release deploy` ([c4947d3](https://www.github.com/werf/nelm/commit/c4947d322fd037eaf9b5ae18a2e6f5a6313ecb23))
* panic in auto internal deps when invalid manifest ([b60fbcc](https://www.github.com/werf/nelm/commit/b60fbccc32fe0a590bedafe88eecc8702ab9d9eb))
* panic with external-dependency namespace annotation ([98f3b27](https://www.github.com/werf/nelm/commit/98f3b272b887b1773b44923fd993ae8d60f5cd13))
* panic with werf.io/replicas-on-creation ([c229357](https://www.github.com/werf/nelm/commit/c2293575a97fadcd04be7fb47b89610da4b07a14))
* parallel GET/dry-APPLY finishing before results received ([e24a1ee](https://www.github.com/werf/nelm/commit/e24a1ee447782cae99c9847a755c3210acc9a285))
* plan build error not showed and deploy graph not saved ([55e3725](https://www.github.com/werf/nelm/commit/55e37254fdb7a09225a7c7ea22c075bdab33b87b))
* **plan:** `hidden sensitive output` message for Secrets even if no sensitive changes ([e6e7926](https://www.github.com/werf/nelm/commit/e6e7926fd144aa5f16439a5087b54baacda98ffd))
* process rollback hooks too ([6af6002](https://www.github.com/werf/nelm/commit/6af600236c67d4d96e3d67fbbf62a91f6816e2be))
* refactor Waiter API ([61695d9](https://www.github.com/werf/nelm/commit/61695d92f231c2fcbf6539f9f83eef2bf31360be))
* **refactor:** new Resource(s), Release, History, ResourcePreparer, KubeClient classes ([9e4dca4](https://www.github.com/werf/nelm/commit/9e4dca4ffdc4ca1773b24e93efd499fbbb0e1bd3))
* release namespace and hook never deleted (except recreation) ([42c9e85](https://www.github.com/werf/nelm/commit/42c9e85cf520362eb1e65e1824cb1bcffaaddf28))
* remove "..." from log messages ([ba0c9c8](https://www.github.com/werf/nelm/commit/ba0c9c852bd8ac740ea4c37bbb98284a43fe0c09))
* remove default stubs from --release-name and --namespace flags ([06ae5e1](https://www.github.com/werf/nelm/commit/06ae5e1f6519ee5d2d6320704a525f4e83cbf4f5))
* remove trailing space from NOTES.txt ([27e5085](https://www.github.com/werf/nelm/commit/27e508528e8d6c3d9095b202248d187cf8e52be3))
* removing chart values with null ([a630e78](https://www.github.com/werf/nelm/commit/a630e7887ba2d59489bcd5a952e8f53eaaa54737))
* replace strategic patch with merge patch ([38d3cbd](https://www.github.com/werf/nelm/commit/38d3cbdb1bbcbcb243e4fb7f0c8396342bc97e5c))
* resources always rendered in no cluster mode ([090b0d1](https://www.github.com/werf/nelm/commit/090b0d1494d39f06b70940ecd862f2052ffd1662))
* return plan even on build error ([46a88cd](https://www.github.com/werf/nelm/commit/46a88cd859d612f407a7550b2b4ead6126865a2e))
* rework release locking in Uninstall action ([ef471b6](https://www.github.com/werf/nelm/commit/ef471b6de46cfc3da464be6d9c7dadb60f750d0f))
* rollback plan error not showed ([7573580](https://www.github.com/werf/nelm/commit/757358010eed5b2d2ff5d56f58c776a0ac38ec48))
* round down columns width to be safe ([de630b1](https://www.github.com/werf/nelm/commit/de630b1be4a250dce8e319704d63a361ee1d449c))
* run `chart lint` in local mode by default ([f385ca7](https://www.github.com/werf/nelm/commit/f385ca77c49f0824f19eadcf7810c7f3a535f1b3))
* show "insignificant changes" in plan if filtered resource diff is ([5c3fba6](https://www.github.com/werf/nelm/commit/5c3fba6e712058be389281720a7e3716ca83c812))
* show live version instead of local for deleted resource diff ([a66fa53](https://www.github.com/werf/nelm/commit/a66fa53f7c15095ebf4b4c9d7effde6feb4a4691))
* simplify executing operations log output ([ed903da](https://www.github.com/werf/nelm/commit/ed903dafe84d5ad37d0da8de19d9dfec22492fd2))
* steal managed fields from any manager with prefix "werf" ([b780c05](https://www.github.com/werf/nelm/commit/b780c054b38d5cf6b92e6c3f74690cde3c19d344))
* switch to go 1.21 ([44e573a](https://www.github.com/werf/nelm/commit/44e573a5d8fe5a68c0feb01248fc3e567dad1ad9))
* tracking might hang with track-termination-mode "NonBlocking" ([c5759ee](https://www.github.com/werf/nelm/commit/c5759eecada45a3687f7db190f687db4d51c9ebb))
* trigger test release ([d0fe732](https://www.github.com/werf/nelm/commit/d0fe732b95e3527c88f974cffc48e4cb35d23524))
* trigger test release /2 ([077682e](https://www.github.com/werf/nelm/commit/077682e8a1802f2e44a20f2068ec6d9314f8c63d))
* twice-deployed pre AND post hooks ([d043cf1](https://www.github.com/werf/nelm/commit/d043cf1bab907b7cad69bc87fd6ca788470c664d))
* update all Go modules ([19e0cb5](https://www.github.com/werf/nelm/commit/19e0cb54756c08ec434095bf9d4d42a227877346))
* update helm and kubedog modules ([53edfcc](https://www.github.com/werf/nelm/commit/53edfccfc0c3b5deeeb06175d74759b9380ccb70))
* update helm dependency ([1398f4a](https://www.github.com/werf/nelm/commit/1398f4abaea750e135ca34eaa18125a46b25c4cb))
* update helm module ([6375703](https://www.github.com/werf/nelm/commit/6375703101bf30c3332125fe6aa9f7be00f7b9e2))
* update jsondiff module ([7009d6b](https://www.github.com/werf/nelm/commit/7009d6b92c5547cad64400f91961f256e8ade884))
* uppercase for STATE column in progress reports ([1647b36](https://www.github.com/werf/nelm/commit/1647b36d41813a4af7d4aafdd4600f4296814a7f))
* **wip:** new resource classes, new release class, history and ([0ef680f](https://www.github.com/werf/nelm/commit/0ef680fd888f248a0b08575427a8fd1bf58497a1))
