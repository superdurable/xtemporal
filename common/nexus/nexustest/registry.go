package nexustest

import (
	"context"

	persistencespb "github.com/superdurable/temporal-server/api/persistence/v1"
	"github.com/superdurable/temporal-server/common/namespace"
	commonnexus "github.com/superdurable/temporal-server/common/nexus"
)

type FakeEndpointRegistry struct {
	OnGetByID   func(ctx context.Context, endpointID string) (*persistencespb.NexusEndpointEntry, error)
	OnGetByName func(ctx context.Context, namespaceID namespace.ID, endpointName string) (*persistencespb.NexusEndpointEntry, error)
}

func (f FakeEndpointRegistry) GetByID(ctx context.Context, endpointID string) (*persistencespb.NexusEndpointEntry, error) {
	return f.OnGetByID(ctx, endpointID)
}

func (f FakeEndpointRegistry) GetByName(ctx context.Context, namespaceID namespace.ID, endpointName string) (*persistencespb.NexusEndpointEntry, error) {
	return f.OnGetByName(ctx, namespaceID, endpointName)
}

func (f FakeEndpointRegistry) StartLifecycle() {
	panic("unimplemented")
}

func (f FakeEndpointRegistry) StopLifecycle() {
	panic("unimplemented")
}

var _ commonnexus.EndpointRegistry = FakeEndpointRegistry{}
