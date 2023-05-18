package common

const (
	// TODO(ilya-lesikov): make it overridable
	ManagedFieldsManager = "helm"
)

type ClientOperationType string

const (
	ClientOperationTypeGet        ClientOperationType = "get"
	ClientOperationTypeCreate     ClientOperationType = "create"
	ClientOperationTypeUpdate     ClientOperationType = "update"
	ClientOperationTypeSmartApply ClientOperationType = "smart-apply"
	ClientOperationTypeDelete     ClientOperationType = "delete"
)
