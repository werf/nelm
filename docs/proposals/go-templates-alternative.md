# Feature: Go templates alternative

## Why

Go templates are fine for simple cases, but scale poorly.

Main issues are:
1. Templating YAML (structured data, sensitive to whitespace) with text templating engine is hard and fundamentally wrong. YAML, as JSON, should be manipulated in a structured way.
1. Go templates are very primitive: even proper functions cannot be defined.
1. The standard Helm library is basic and cannot be extended by the end user. Third-party function libraries are not possible.
1. Lots of gotchas, like `{{ if (include "always-returns-false" .) }}` will always be true.
1. Debugging complex Helm templates is notoriously difficult.
1. Poor tooling support (IDEs, linters, etc.).
1. Issues with performance.
1. Issues with mutability of Values and more.

## What

Nelm should provide a way to generate manifests in a scalable way. So that it is easier to write, read, debug and test the manifest generation logic.

## Possible solutions

### Post-renderers

Rejected.

Helm has post-renderers, but they are a poor fit to have an alternative to Go templates or improve them, because:
1. Post-renderers need to be shipped and installed separately, as plugins.
1. Additional binaries and other files, like shared libs, might need to be installed separately.
1. Post-renderers might require configuration.
1. No normal way to go into subcharts to get files that post-renderers might need. This is because post-renderers know only about rendered manifests, not about charts or values.
1. There might be dozens of different post-renderers, each with its own way of doing things. It doesn't exactly help when a single release might require multiple different post-renderers for its subcharts.

### Helm charts as WASM binaries

Rejected.

An application written in any language can be packaged as a WASM binary, and then treated as a Helm chart. This WASM binary must accept values on stdin and return rendered manifests on stdout, so that Nelm will run it to render manifests. Main issues here:
1. An option to write charts in any language will lead to serious fragmentation. Now you suddenly need to know many different languages to work with different charts.
1. If the Helm chart is a WASM binary, it's in a non-editable and a non-versionable format. It cannot be pulled, edited, and deployed.
1. We cannot provide a proper integration (SDKs, ...) to dozens of different languages.

### Improve Go templates

Rejected.

Text templating of YAML is fundamentally broken. Too many aspects of Go templates cannot be fixed without breaking backward compatibility. Improving besides adding new functions is difficult and will require forking of `text/template` package of Go standard library.

### Support another text templating engine

Rejected.

Using another text templating engine, e.g. Jinja, does not solve the fundamental issue of trying to template structured data in a non-structured way. Most other issues, like lack of third-party libraries and difficult debugging, still apply.

### Support a configuration language or a general programming language

Preferred.

Hard requirements for the solution:
1. Minimal abandonment risk.
1. Mature.
1. No-dependency/no-CGO embedding into Nelm.
1. No-dependency/no-configuration deploys, like with Go templates.
1. Air-gapped deploys.
1. No more than one new language, otherwise too much fragmentation.

Here is a comparison of all viable options, along with Go templates for reference:

|  | gotpl | ts | python | go | cue | kcl | pkl | jsonnet | ytt | starlark | dhall |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| Activity | Active | Active | Active | Active | Active | Active | Active | Maintenance | Abandoned | Abandoned | Abandoned |
| Abandonment risk¹ | No | No | No | No | Moderate | High | Moderate |  |  |  |  |
| Maturity | Great | Great | Great | Great | Good | Moderate | Poor |  |  |  |  |
| Zero-dep embedding² | Yes | Yes | Poor | No | Yes | No | No |  |  |  |  |
| Libs management | Poor | Yes | Yes | Yes | Yes | Yes | No |  |  |  |  |
| Libs bundling³ | No | Yes | No | No | No | No | No |  |  |  |  |
| Air-gapped deploys⁴ | Poor | Yes | Poor | Poor | Poor | Poor | No |  |  |  |  |
| 3rd-party libraries | Few | Great | Great | Great | Few | No | No |  |  |  |  |
| Tooling (editors, ...)| Poor | Great | Great | Great | Poor |  |  |  |  |  |  |
| Working with CRs | Poor | Great | Great | Poor | Great |  |  |  |  |  |  |
| Complexity | 2 | 4 | 2 | 3 | 3 |  |  |  |  |  |  |
| Flexibility | 2 | 5 | 4 | 3 | 2 |  |  |  |  |  |  |
| Debugging | 1 | 5 | 5 | 5 | 2 |  |  |  |  |  |  |
| Community | 2 | 5 | 5 | 5 | 1 | 1 | 1 |  |  |  |  |
| Determinism | Possible | Possible | Possible | Possible | Yes | Possible | Possible |  |  |  |  |
| Hermeticity | No | Yes | Yes | Yes | Yes | No | No |  |  |  |  |

(1) Abandonment risk:
1. At least "medium" for specialized configuration languages, like Jsonnet or CUE. These languages are not widely adopted, have small communities, and are often end up abandoned.
1. The number of active maintainers is a factor.
1. The complexity of the language and its tooling is a factor.

(2) Zero-dependency embedding into Nelm:
1. TS: esbuild + Goja (native), esbuild + QuickJS + Wazero (WASM).
1. Python: Pyodide (WASM), Gpython (native). Gpython is abandoned.
1. Go: yaegi (native). Yaegi is abandoned, supports Go 1.21-1.22 only.
1. CUE: native Go lib.
1. KCL, Pkl: not possible, requires binary.

(3) Libraries bundling:
1. TS: embedded esbuild can bundle everything into a single JS file.
1. Python: probably not possible to bundle everything into a single file. C extensions won't work.
1. Go: yaegi doesn't support external libraries at all, unless compiled into the Nelm binary.
1. CUE, KCL, Pkl: no tooling for this.

(4) Air gapped deploys:
1. TS: with embed esbuild all deps bundled on chart push, no network access needed on deploy.
1. Others: theoretically possible, practically not doable.

### Preferred solution

Adding support for a new language is the only solution. TypeScript seems like the best fit. Main challenges with TS are complexity for non-developers and chart fragmentation due to now having two ways to write charts.

Complexity for non-developers can be lowered by:
1. Option to forbid classes declaration, parallelism, etc (eslint).
1. Option to forbid complex 3rd-party libraries like cdk8s (eslint).
1. A separate command like `nelm chart create`, which creates all necessary directories, files and configuration.
1. Keep TS in Nelm as vanilla as possible, so that the chart can be developed as a normal standalone TS project.

Stick to Go templates if you don't have a lot of charts or if you are unsure if your team can handle TS. But if you go for TS, the complexity of supporting charts in two languages can be lowered by:
1. Write TS charts for internal use, e.g. for business app charts.
1. You can use third-party Go template charts for supporting software, such as databases.
1. When the Go template chart is missing something — patch it on the fly with Nelm (the feature is not implemented yet, but it will be).
1. If patching isn't enough, and you want to rewrite the Go template chart — rewrite it in TS instead.
This way you don't have to work with Go templates anymore and can focus on TS.

## Design

The chart structure with TypeScript support will look like this:
```
Chart.yaml
values.yaml
templates/
ts/
  package.json
  tsconfig.json
  node_modules/
  vendor/
  src/
    index.ts
    deployment.ts
    service.ts
```

Here the only difference is the new `ts` directory. This directory basically represents a TypeScript application. The application must accept Helm root context data (values, chart info, etc.) in YAML as input and return rendered manifests as output. The application must be possible to run with `npm run ...`.

During chart publishing all dependencies will be bundled into `vendor/libs.js` (source maps embedded) and `node_modules` directory will not be published. This will be done with esbuild, which is to be embedded in Nelm.

Nelm must embed [esbuild](https://github.com/evanw/esbuild) (TS/JS transpiler and bundler) and [Goja](https://github.com/dop251/goja/).

Development workflow:
1. NodeJS, NPM and Nelm must be installed for local development.
1. The command `nelm chart create` creates a Chart with the `ts` directory and all the boilerplate files.
1. Open `ts` directory in your IDE/editor as a TypeScript NodeJS project and work like you would with a regular TS project.
1. (optional) Run `npm run ...` to execute application with NodeJS runtime and render manifests to stdout.
1. (optional) Run `npm run ...` to run tests.
1. (optional) Run `nelm chart render/lint` to check if the code actually works under Goja runtime.
1. Run `nelm chart upload` to publish the chart.

Deployment workflow:
1. Only Nelm must be installed. No NodeJS, npm, esbuild or anything else needed.
1. Command `nelm release install myrepo/mychart` installs previously published chart as usual.

During `nelm release install` Nelm will grab `ts/vendor/libs.js` and `ts/src` files and transpile them to JS with embedded esbuild. Then JS code will be passed to the embedded JS-runtime Goja and executed. This will render manifests, after which these manifests are appended to templated manifests from `templates` directory. Then everything deployed together, as usual.

Nelm must provide a TS SDK with input (values, ...) and output (manifests) types defined. SDK should not be embedded in Nelm. It will be published as a separate NPM package, and will be specified in `package.json` along with [Node.js compatibility layer for Goja](https://github.com/dop251/goja_nodejs).
