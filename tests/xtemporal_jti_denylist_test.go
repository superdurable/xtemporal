package tests

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/api"
	"github.com/superdurable/xtemporal/common/authorization"
	"github.com/superdurable/xtemporal/common/config"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
	"github.com/superdurable/xtemporal/x/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type xtJTIKeyProvider struct{ key *rsa.PublicKey }

func (p xtJTIKeyProvider) RsaKey(_, _ string) (*rsa.PublicKey, error)     { return p.key, nil }
func (p xtJTIKeyProvider) EcdsaKey(_, _ string) (*ecdsa.PublicKey, error) { return nil, nil }
func (p xtJTIKeyProvider) HmacKey(_, _ string) ([]byte, error)            { return nil, nil }
func (p xtJTIKeyProvider) SupportedMethods() []string                     { return []string{jwt.SigningMethodRS256.Alg()} }
func (p xtJTIKeyProvider) Close()                                         {}

func TestXTJTIRevocation(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "denylist.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(`{"jti":"revoked","exp":9999999999}`+"\n"), 0600))
	denylist := auth.NewDenylist(auth.FileSource{Path: path}, log.NewNoopLogger(), metrics.NoopMetricsHandler, true)
	denylist.Reload(context.Background())
	base := authorization.NewDefaultJWTClaimMapper(xtJTIKeyProvider{key: &key.PublicKey}, &config.Authorization{}, log.NewNoopLogger())
	mapper := auth.NewMapper(base, denylist)

	for _, tc := range []struct {
		jti     string
		revoked bool
	}{
		{"revoked", true},
		{"active", false},
	} {
		t.Run(tc.jti, func(t *testing.T) {
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
				"sub": "api-key", "jti": tc.jti, "exp": time.Now().Add(time.Hour).Unix(),
				"permissions": []string{"default:read"},
			})
			token.Header["kid"] = "current"
			signed, err := token.SignedString(key)
			require.NoError(t, err)
			claims, err := mapper.GetClaims(&authorization.AuthInfo{AuthToken: "Bearer " + signed})
			if tc.revoked {
				require.Equal(t, codes.Unauthenticated, status.Code(err))
				require.Nil(t, claims)
			} else {
				require.NoError(t, err)
				require.Equal(t, "api-key", claims.Subject)
				require.Equal(t, authorization.RoleReader, claims.Namespaces["default"])
			}
		})
	}
	authorizer, err := auth.GetAuthorizerFromConfig(&config.Authorization{Authorizer: "platformNamespaceLifecycle"})
	require.NoError(t, err)
	for _, tc := range []struct {
		name       string
		permission string
		apiName    string
		decision   authorization.Decision
	}{
		{"namespace admin register", "default:admin", api.WorkflowServicePrefix + "RegisterNamespace", authorization.DecisionDeny},
		{"namespace admin delete", "default:admin", api.OperatorServicePrefix + "DeleteNamespace", authorization.DecisionDeny},
		{"namespace admin update", "default:admin", api.WorkflowServicePrefix + "UpdateNamespace", authorization.DecisionDeny},
		{"namespace admin deprecate", "default:admin", api.WorkflowServicePrefix + "DeprecateNamespace", authorization.DecisionDeny},
		{"namespace admin add search attributes", "default:admin", api.OperatorServicePrefix + "AddSearchAttributes", authorization.DecisionAllow},
		{"system admin register", "temporal-system:admin", api.WorkflowServicePrefix + "RegisterNamespace", authorization.DecisionAllow},
		{"system admin delete", "temporal-system:admin", api.OperatorServicePrefix + "DeleteNamespace", authorization.DecisionAllow},
		{"system admin update", "temporal-system:admin", api.WorkflowServicePrefix + "UpdateNamespace", authorization.DecisionAllow},
		{"system admin deprecate", "temporal-system:admin", api.WorkflowServicePrefix + "DeprecateNamespace", authorization.DecisionAllow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
				"sub": "api-key", "jti": "active", "exp": time.Now().Add(time.Hour).Unix(),
				"permissions": []string{tc.permission},
			})
			token.Header["kid"] = "current"
			signed, err := token.SignedString(key)
			require.NoError(t, err)
			claims, err := mapper.GetClaims(&authorization.AuthInfo{AuthToken: "Bearer " + signed})
			require.NoError(t, err)
			result, err := authorizer.Authorize(context.Background(), claims, &authorization.CallTarget{APIName: tc.apiName, Namespace: "default"})
			require.NoError(t, err)
			require.Equal(t, tc.decision, result.Decision)
		})
	}
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	forged := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "api-key", "jti": "active", "exp": time.Now().Add(time.Hour).Unix(),
	})
	forged.Header["kid"] = "current"
	signed, err := forged.SignedString(otherKey)
	require.NoError(t, err)
	claims, err := mapper.GetClaims(&authorization.AuthInfo{AuthToken: "Bearer " + signed})
	require.Error(t, err)
	require.Nil(t, claims)
}
