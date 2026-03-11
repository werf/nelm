package kube

import "k8s.io/cli-runtime/pkg/resource"

type ClientExtender interface {
	BeforeCreateResource(info *resource.Info) error
	BeforeUpdateResource(info *resource.Info) error
	BeforeDeleteResource(info *resource.Info) error
}
