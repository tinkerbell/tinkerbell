/*
Package datamodel defines constants for the various back-end storage mechanisms supported by hegel.
*/
package datamodel

// DataModel defines the mechanism for back-end data retrieval.
type DataModel string

const (
	TinkServer DataModel = "1"
	Kubernetes DataModel = "kubernetes"
)
