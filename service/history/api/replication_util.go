package api

import (
	"github.com/superdurable/xtemporal/common/cluster"
	"go.temporal.io/api/serviceerror"
)

func ValidateReplicationConfig(
	clusterMetadata cluster.Metadata,
) error {
	if !clusterMetadata.IsGlobalNamespaceEnabled() {
		return serviceerror.NewUnavailable("The cluster has global namespace disabled. The operation is not supported.")
	}
	return nil
}
