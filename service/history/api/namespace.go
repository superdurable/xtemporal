package api

import (
	"github.com/google/uuid"
	"go.temporal.io/api/serviceerror"
	"github.com/superdurable/xtemporal/common/namespace"
	historyi "github.com/superdurable/xtemporal/service/history/interfaces"
)

func GetActiveNamespace(
	shard historyi.ShardContext,
	namespaceUUID namespace.ID,
	businessID string,
) (*namespace.Namespace, error) {

	err := ValidateNamespaceUUID(namespaceUUID)
	if err != nil {
		return nil, err
	}

	namespaceEntry, err := shard.GetNamespaceRegistry().GetNamespaceByID(namespaceUUID)
	if err != nil {
		return nil, err
	}
	if namespaceEntry.ActiveClusterName(namespace.RoutingKey{ID: businessID}) != shard.GetClusterMetadata().GetCurrentClusterName() {
		return nil, serviceerror.NewNamespaceNotActive(
			namespaceEntry.Name().String(),
			shard.GetClusterMetadata().GetCurrentClusterName(),
			namespaceEntry.ActiveClusterName(namespace.RoutingKey{ID: businessID}))
	}
	return namespaceEntry, nil
}

func GetNamespace(
	shard historyi.ShardContext,
	namespaceUUID namespace.ID,
) (*namespace.Namespace, error) {

	err := ValidateNamespaceUUID(namespaceUUID)
	if err != nil {
		return nil, err
	}

	namespaceEntry, err := shard.GetNamespaceRegistry().GetNamespaceByID(namespaceUUID)
	if err != nil {
		return nil, err
	}

	return namespaceEntry, nil
}

func ValidateNamespaceUUID(
	namespaceUUID namespace.ID,
) error {
	if namespaceUUID == "" {
		return serviceerror.NewInvalidArgument("Missing namespace UUID.")
	} else if uuid.Validate(namespaceUUID.String()) != nil {
		return serviceerror.NewInvalidArgument("Invalid namespace UUID.")
	}
	return nil
}
