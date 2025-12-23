<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Overview](#overview)
- [Dependencies](#dependencies)
- [Architecture](#architecture)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Overview

Nelm originally was just a werf deployment engine, but eventually got its own repo and CLI. Nelm is reusing parts of the Helm codebase, but most of its codebase is written from scratch.

## Dependencies

Nelm depends on the following projects from the "werf" organization:

* https://github.com/werf/3p-helm is a fork of Helm with some modifications. We use some parts of this fork in Nelm.
* https://github.com/werf/kubedog is a library for tracking Kubernetes resource statuses, collecting logs and events during Nelm deployments.
* https://github.com/werf/common-go is a library with some common code shared between various werf projects.

Nelm is pretty straightforward: no frameworks, no CGO, no code generation, and libraries we use are very common (e.g. Cobra for CLI). For the full list of dependencies look into the `go.mod` file.

## Architecture

TODO
