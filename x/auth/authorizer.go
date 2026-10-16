package auth

import (
	"context"
	"strings"

	"github.com/superdurable/xtemporal/common/api"
	"github.com/superdurable/xtemporal/common/authorization"
	"github.com/superdurable/xtemporal/common/config"
)

type platformNamespaceLifecycleAuthorizer struct {
	delegate authorization.Authorizer
}

func GetAuthorizerFromConfig(cfg *config.Authorization) (authorization.Authorizer, error) {
	if !strings.EqualFold(cfg.Authorizer, "platformNamespaceLifecycle") {
		return authorization.GetAuthorizerFromConfig(cfg)
	}
	return &platformNamespaceLifecycleAuthorizer{delegate: authorization.NewDefaultAuthorizer()}, nil
}

func (a *platformNamespaceLifecycleAuthorizer) Authorize(ctx context.Context, claims *authorization.Claims, target *authorization.CallTarget) (authorization.Result, error) {
	if (target.APIName == api.OperatorServicePrefix+"DeleteNamespace" ||
		target.APIName == api.WorkflowServicePrefix+"RegisterNamespace" ||
		target.APIName == api.WorkflowServicePrefix+"UpdateNamespace" ||
		target.APIName == api.WorkflowServicePrefix+"DeprecateNamespace") &&
		(claims == nil || claims.System == authorization.RoleUndefined) {
		return authorization.Result{Decision: authorization.DecisionDeny}, nil
	}
	return a.delegate.Authorize(ctx, claims, target)
}
