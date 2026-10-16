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
		name            string
		claims          *authorization.Claims
		apiName         string
		targetNamespace string
		decision        authorization.Decision
	}{
		{"namespace admin register", namespaceAdmin, api.WorkflowServicePrefix + "RegisterNamespace", "test", authorization.DecisionDeny},
		{"namespace admin update", namespaceAdmin, api.WorkflowServicePrefix + "UpdateNamespace", "test", authorization.DecisionDeny},
		{"namespace admin delete", namespaceAdmin, api.OperatorServicePrefix + "DeleteNamespace", "test", authorization.DecisionDeny},
		{"namespace admin deprecate", namespaceAdmin, api.WorkflowServicePrefix + "DeprecateNamespace", "test", authorization.DecisionDeny},
		{"namespace admin add search attributes in own namespace", namespaceAdmin, api.OperatorServicePrefix + "AddSearchAttributes", "test", authorization.DecisionAllow},
		{"namespace admin remove search attributes in own namespace", namespaceAdmin, api.OperatorServicePrefix + "RemoveSearchAttributes", "test", authorization.DecisionAllow},
		{"namespace admin list search attributes in own namespace", namespaceAdmin, api.OperatorServicePrefix + "ListSearchAttributes", "test", authorization.DecisionAllow},
		{"namespace admin add search attributes in other namespace", namespaceAdmin, api.OperatorServicePrefix + "AddSearchAttributes", "other", authorization.DecisionDeny},
		{"namespace admin create nexus endpoint", namespaceAdmin, api.OperatorServicePrefix + "CreateNexusEndpoint", "test", authorization.DecisionDeny},
		{"namespace admin add or update remote cluster", namespaceAdmin, api.OperatorServicePrefix + "AddOrUpdateRemoteCluster", "test", authorization.DecisionDeny},
		{"namespace admin admin service get shard", namespaceAdmin, api.AdminServicePrefix + "GetShard", "test", authorization.DecisionDeny},
		{"namespace admin admin service describe cluster", namespaceAdmin, api.AdminServicePrefix + "DescribeCluster", "test", authorization.DecisionDeny},
		{"namespace writer start workflow", namespaceWriter, api.WorkflowServicePrefix + "StartWorkflowExecution", "test", authorization.DecisionAllow},
		{"namespace writer add search attributes", namespaceWriter, api.OperatorServicePrefix + "AddSearchAttributes", "test", authorization.DecisionDeny},
		{"system admin register", systemAdmin, api.WorkflowServicePrefix + "RegisterNamespace", "test", authorization.DecisionAllow},
		{"system admin update", systemAdmin, api.WorkflowServicePrefix + "UpdateNamespace", "test", authorization.DecisionAllow},
		{"system admin delete", systemAdmin, api.OperatorServicePrefix + "DeleteNamespace", "test", authorization.DecisionAllow},
		{"system admin deprecate", systemAdmin, api.WorkflowServicePrefix + "DeprecateNamespace", "test", authorization.DecisionAllow},
		{"nil claims register", nil, api.WorkflowServicePrefix + "RegisterNamespace", "test", authorization.DecisionDeny},
		{"nil claims update", nil, api.WorkflowServicePrefix + "UpdateNamespace", "test", authorization.DecisionDeny},
		{"nil claims delete", nil, api.OperatorServicePrefix + "DeleteNamespace", "test", authorization.DecisionDeny},
		{"nil claims deprecate", nil, api.WorkflowServicePrefix + "DeprecateNamespace", "test", authorization.DecisionDeny},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := authorizer.Authorize(context.Background(), tc.claims, &authorization.CallTarget{APIName: tc.apiName, Namespace: tc.targetNamespace})
			require.NoError(t, err)
			require.Equal(t, tc.decision, result.Decision)
		})
	}
}

func TestPlatformNamespaceLifecycleAuthorizerMatchesFullAPIName(t *testing.T) {
	claims := &authorization.Claims{Namespaces: map[string]authorization.Role{"test": authorization.RoleAdmin}}
	for _, apiName := range []string{
		api.WorkflowServicePrefix + "DeleteNamespace",
		api.OperatorServicePrefix + "RegisterNamespace",
		api.WorkflowServicePrefix + "RegisterNamespaceExtra",
	} {
		t.Run(apiName, func(t *testing.T) {
			delegate := &recordingAuthorizer{}
			authorizer := &platformNamespaceLifecycleAuthorizer{delegate: delegate}
			result, err := authorizer.Authorize(context.Background(), claims, &authorization.CallTarget{
				APIName:   apiName,
				Namespace: "test",
			})
			require.NoError(t, err)
			require.Equal(t, authorization.DecisionAllow, result.Decision)
			require.True(t, delegate.called)
		})
	}
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
