# Feature: Go templates alternative

## Why

Go templates are fine for simple cases, but scale poorly:
1. Templating YAML (structured data, sensitive to whitespace) with text templating engine is hard and fundamentally wrong. YAML, as JSON, should be manipulated in a structured way.
1. Go templates are very primitive: even proper functions cannot be defined.
1. The standard Helm library is basic and cannot be extended by the end user. Third-party function libraries are not possible.
1. Lots of gotchas, like `{{ if (include "always-returns-false" .) }}` will always be true.
1. Debugging complex Helm templates is notoriously difficult.
1. Poor tooling support (IDEs, linters, etc.).
1. Issues with performance.
1. Issues with mutability of Values and more.

## What

Nelm should provide an alternative to Go templates for generating or templating Kubernetes manifests.

Helm only supports Go templates. It also has post-renderers, but they are a poor fit for an alternative to Go templates, because:
1. They need to be shipped and installed as plugins.
1. Binaries might need to be installed separately.
1. They might have system dependencies.
1. They might require configuration.
1. Some clever magic needed to go into subcharts to get files they need to render manifests. This is because they know only about rendered manifests, not about charts or values.
1. There might be dozens of different post-renderers, each with its own way of doing things. It doesn't exactly help when a single release might require multiple different post-renderers for its subcharts.

Helm succeeded because of Helm charts. And the success of Helm charts is in Go templating. Not because it's good, but because it's simple: no dependencies, no configuration, no alternative.

Helm 2 made it possible to implement alternative templating engines in Helm core. [No one used it](https://github.com/helm/helm/issues/9855#issuecomment-867565430).

Another option would be to package an application written in any language as a WASM binary, and treat it as a Helm chart basically. This WASM binary must accept values on stdin and return rendered manifests on stdout, so that Nelm will run it to render manifests. Even if this can be implemented, the issue stays: no one is going to work with charts in dozens of different languages.

So what we do? We can provide an alternative to Go templates, but it must be:
1. Single language (or maybe two, at most), not dozens.
1. Embedded in Nelm.
1. No plugins.
1. No additional binaries.
1. No additional system dependencies.
1. No configuration.
1. Receive values and its source files as an input.
1. Return rendered manifests.
1. Work on a per-chart basis, respect chart dependencies.
1. Maintain feature parity with Go templates.

## Possible solutions

### Improving Go templates

Rejected.

Text templating of YAML is fundamentally broken. Too many aspects of Go templates cannot be fixed without breaking backward compatibility. Improving besides adding new functions is difficult.

### Using another text templating engine

Rejected.

Using another text templating engine, e.g. Jinja, does not solve the fundamental issue of trying to template structured data in a non-structured way. Most other issues, like lack of third-party libraries and difficult debugging, still apply.

### Using a configuration language

Not decided yet.

Specialized configuration languages, like Jsonnet or CUE, are designed to work with structured data. They solve some issues of Go templates, but not all of them, namely:
1. Still not as flexible as general-purpose programming languages (Jsonnet). Sometimes not Turing-complete (CUE).
1. Usually no support for third-party libraries or a very limited number of libraries.
1. Poor tooling support (IDEs/editors, etc.).
1. Often issues with debugging.
1. Often issues with performance.

On top of this, configuration languages have their own drawbacks:
1. Some are weird, making adoption and onboarding difficult (CUE).
1. Generally poor adoption.
1. Small community, lack of learning resources.
1. Easily might end up abandoned.

### Using a general-purpose language

Preferred.

In comparison to configuration languages, a popular general-purpose programming language like TypeScript has these advantages:
1. Very flexible and powerful. Good typing system.
1. Thousands of third-party libraries, including specialized (cdk8s, kubernetes client, etc.).
1. Great tooling support (IDEs/editors, linters, formatters, etc.).
1. Alright dependency management.
1. Good, stable performance.
1. Conventional, not weird (like CUE).
1. Easy debugging.
1. Easy testing.
1. Mature, proven.
1. Wide adoption.
1. Big community, lots of learning resources.
1. Not going to be abandoned any time soon.
1. Useful skill to learn in general. Can be used for other purposes.

Cons:
1. More difficult to learn if no prior programming experience.
1. Can be too flexible, making things complicated.
1. Hermeticity and determinism not guaranteed. Require skills, discipline, tooling.
1. Complicated tooling, e.g. package managers and build systems.
1. Less secure, less isolated (mitigated by WASM).

### Currently preferred solution

It seems that if you need to pick only one alternative to Go templates, a general-purpose language will provide more value. Configuration languages suffer from many of the same issues as Go templates.

If you look at this in terms of scalability, then:
* Go templates are poorly scalable, but conventional, widely adopted and easy to use for simple cases.
* Configuration languages are moderately scalable, but unconventional, poorly adopted and more complicated.
* General-purpose languages are highly scalable, conventional, widely adopted, but the most complicated.

Makes more sense to provide very scalable + non-scalable options, rather than moderately scalable + non-scalable. This way users can start easy (Helm templates) and then move to highly scalable (TypeScript) when they really struggle. Any such migration is a big resource sink, so I honestly don't see that much value in migrating to Jsonnet or CUE when the user already does everything in Helm templates.

We evaluated TypeScript, and it seems fit (pros and cons listed above).

## Specification

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

Here the only change is that the new `ts` directory is added. This directory basically represents a TypeScript application. The application must accept Helm root context data (values, chart info, etc.) as input and return rendered manifests as output. The application must be possible to run with `npm start`.

`node_modules` directory must be in .gitignore. During chart publishing all dependencies must be bundled into `vendor/libs.js`, which can be done with esbuild, which is to be embedded in Nelm.

Nelm must include Wazero (WASM runtime), QuickJS (JS runtime) and [esbuild](https://github.com/evanw/esbuild) (JS transpiler and bundler). [QJS](https://github.com/fastschema/qjs) might work for Wazero + QuickJS.

Development workflow:
1. NodeJS, NPM and Nelm must be installed for local development.
1. Command `nelm chart ts init .` creates `ts` directory with boilerplate files.
1. `ts` directory opened as a TypeScript NodeJS project in IDE/editor.
1. Work as you would with a TypeScript project.
1. Run `npm TODO` to execute application with NodeJS runtime and render manifests to stdout.
1. Run `npm TODO` to run tests.
1. Run `nelm chart render` to render manifests with the QuickJS runtime.
1. Run `nelm chart upload` to publish the chart. Nelm will bundle dependencies into `vendor/libs.js` during publishing with embedded esbuild.

Deployment workflow:
1. Only Nelm must be installed. No NodeJS, QuickJS, npm, esbuild or anything else needed.
1. Command `nelm release install myrepo/mychart` installs previously published chart as usual.
1. Under the hood, Nelm will grab dependencies from `ts/vendor` and transpile `ts/src/index.ts` to JS with embedded esbuild, then pass JS code to embedded QuickJS runtime working in WASM via Wazero. This will render manifests that will be appended to templated manifests from `templates` directory. Then everything will be deployed as usual.

QuickJS probably doesn't have the best tooling support, so you are going to develop with NodeJS. But QuickJS doesn't support everything NodeJS does. Mitigation: `nelm chart render/lint` will help with testing the code under QuickJS.

WASM has capabilities for network/fs isolation for better security and reproducibility.

## Links
