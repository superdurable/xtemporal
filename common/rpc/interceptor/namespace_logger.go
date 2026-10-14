package interceptor

import (
	"context"
	"crypto/md5"
	"fmt"

	"github.com/superdurable/xtemporal/common/api"
	"github.com/superdurable/xtemporal/common/authorization"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/log/tag"
	"github.com/superdurable/xtemporal/common/namespace"
	"google.golang.org/grpc"
)

type (
	NamespaceLogInterceptor struct {
		namespaceRegistry namespace.Registry
		logger            log.Logger
	}
)

var _ grpc.UnaryServerInterceptor = (*NamespaceLogInterceptor)(nil).Intercept

func NewNamespaceLogInterceptor(namespaceRegistry namespace.Registry, logger log.Logger) *NamespaceLogInterceptor {

	return &NamespaceLogInterceptor{
		namespaceRegistry: namespaceRegistry,
		logger:            logger,
	}
}

func (nli *NamespaceLogInterceptor) Intercept(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {

	if nli.logger != nil {
		methodName := api.MethodName(info.FullMethod)
		namespace := MustGetNamespaceName(nli.namespaceRegistry, req)
		tlsInfo := authorization.TLSInfoFromContext(ctx)
		var serverName string
		var certThumbprint string
		if tlsInfo != nil {
			serverName = tlsInfo.State.ServerName
			cert := authorization.PeerCert(tlsInfo)
			if cert != nil {
				certThumbprint = fmt.Sprintf("%x", md5.Sum(cert.Raw))
			}
		}
		nli.logger.Debug(
			"Frontend method invoked.",
			tag.WorkflowNamespace(namespace.String()),
			tag.Operation(methodName),
			tag.ServerName(serverName),
			tag.CertThumbprint(certThumbprint))
	}
	return handler(ctx, req)
}
