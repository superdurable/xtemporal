package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/api"
	"github.com/superdurable/xtemporal/common/authorization"
	"github.com/superdurable/xtemporal/common/config"
)

func TestPlatformNamespaceLifecycleAuthorizer(t *testing.T) {
	authorizer, err := GetAuthorizerFromConfig(&config.Authorization{Authorizer: "platformNamespaceLifecycle"})
	require.NoError(t, err)

	namespaceAdmin := &authorization.Claims{Namespaces: map[string]authorization.Role{"test": authorization.RoleAdmin}}
	namespaceWriter := &authorization.Claims{Namespaces: map[string]authorization.Role{"test": authorization.RoleWriter}}
	systemAdmin := &authorization.Claims{System: authorization.RoleAdmin}
	for _, tc := range []struct {
		name     string
		claims   *authorization.Claims
		apiName  string
		decision authorization.Decision
	}{
		{"namespace admin delete", namespaceAdmin, api.OperatorServicePrefix + "DeleteNamespace", authorization.DecisionDeny},
		{"namespace admin update", namespaceAdmin, api.WorkflowServicePrefix + "UpdateNamespace", authorization.DecisionDeny},
		{"namespace admin deprecate", namespaceAdmin, api.WorkflowServicePrefix + "DeprecateNamespace", authorization.DecisionDeny},
		{"namespace admin add search attributes", namespaceAdmin, api.OperatorServicePrefix + "AddSearchAttributes", authorization.DecisionAllow},
		{"namespace admin remove search attributes", namespaceAdmin, api.OperatorServicePrefix + "RemoveSearchAttributes", authorization.DecisionAllow},
		{"namespace admin register", namespaceAdmin, api.WorkflowServicePrefix + "RegisterNamespace", authorization.DecisionAllow},
		{"namespace writer start workflow", namespaceWriter, api.WorkflowServicePrefix + "StartWorkflowExecution", authorization.DecisionAllow},
		{"namespace writer add search attributes", namespaceWriter, api.OperatorServicePrefix + "AddSearchAttributes", authorization.DecisionDeny},
		{"system admin delete", systemAdmin, api.OperatorServicePrefix + "DeleteNamespace", authorization.DecisionAllow},
		{"system admin update", systemAdmin, api.WorkflowServicePrefix + "UpdateNamespace", authorization.DecisionAllow},
		{"system admin deprecate", systemAdmin, api.WorkflowServicePrefix + "DeprecateNamespace", authorization.DecisionAllow},
		{"nil claims delete", nil, api.OperatorServicePrefix + "DeleteNamespace", authorization.DecisionDeny},
		{"nil claims update", nil, api.WorkflowServicePrefix + "UpdateNamespace", authorization.DecisionDeny},
		{"nil claims deprecate", nil, api.WorkflowServicePrefix + "DeprecateNamespace", authorization.DecisionDeny},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := authorizer.Authorize(context.Background(), tc.claims, &authorization.CallTarget{APIName: tc.apiName, Namespace: "test"})
			require.NoError(t, err)
			require.Equal(t, tc.decision, result.Decision)
		})
	}
}

func TestPlatformNamespaceLifecycleAuthorizerMatchesFullAPIName(t *testing.T) {
	delegate := &recordingAuthorizer{}
	authorizer := &platformNamespaceLifecycleAuthorizer{delegate: delegate}
	claims := &authorization.Claims{Namespaces: map[string]authorization.Role{"test": authorization.RoleAdmin}}
	result, err := authorizer.Authorize(context.Background(), claims, &authorization.CallTarget{
		APIName:   api.WorkflowServicePrefix + "DeleteNamespace",
		Namespace: "test",
	})
	require.NoError(t, err)
	require.Equal(t, authorization.DecisionAllow, result.Decision)
	require.True(t, delegate.called)
}

func TestGetAuthorizerFromConfigDelegates(t *testing.T) {
	for _, name := range []string{"", "default", "unknown"} {
		t.Run(name, func(t *testing.T) {
			cfg := &config.Authorization{Authorizer: name}
			got, gotErr := GetAuthorizerFromConfig(cfg)
			want, wantErr := authorization.GetAuthorizerFromConfig(cfg)
			require.Equal(t, wantErr, gotErr)
			require.Equal(t, want, got)
		})
	}
}

type recordingAuthorizer struct{ called bool }

func (a *recordingAuthorizer) Authorize(context.Context, *authorization.Claims, *authorization.CallTarget) (authorization.Result, error) {
	a.called = true
	return authorization.Result{Decision: authorization.DecisionAllow}, nil
}
