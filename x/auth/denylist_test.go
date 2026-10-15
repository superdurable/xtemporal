package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/authorization"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/metrics"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type claimMapperFunc func(*authorization.AuthInfo) (*authorization.Claims, error)

func (f claimMapperFunc) GetClaims(info *authorization.AuthInfo) (*authorization.Claims, error) {
	return f(info)
}

func testToken(t *testing.T, jti string, includeJTI bool) *authorization.AuthInfo {
	t.Helper()
	claims := jwt.MapClaims{"sub": "user", "exp": time.Now().Add(time.Hour).Unix()}
	if includeJTI {
		claims["jti"] = jti
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret"))
	require.NoError(t, err)
	return &authorization.AuthInfo{AuthToken: "Bearer " + token}
}

func TestMapper(t *testing.T) {
	path := filepath.Join(t.TempDir(), "denylist.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(`{"jti":"revoked","exp":9999999999}`+"\n"+`{"jti":"expired","exp":1}`+"\n"), 0600))
	loaded, err := (FileSource{Path: path}).Load(context.Background())
	require.NoError(t, err)
	require.NotContains(t, loaded, "expired")
	denylist := NewDenylist(FileSource{Path: path}, log.NewNoopLogger(), metrics.NoopMetricsHandler, true)
	denylist.Reload(context.Background())
	delegate := claimMapperFunc(func(*authorization.AuthInfo) (*authorization.Claims, error) {
		return &authorization.Claims{Subject: "user"}, nil
	})
	mapper := NewMapper(delegate, denylist)
	for _, tc := range []struct {
		name       string
		jti        string
		includeJTI bool
		rejected   bool
	}{
		{"listed", "revoked", true, true},
		{"not listed", "active", true, false},
		{"missing jti", "", false, true},
		{"expired entry pruned", "expired", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := mapper.GetClaims(testToken(t, tc.jti, tc.includeJTI))
			if tc.rejected {
				require.Equal(t, codes.Unauthenticated, status.Code(err))
				require.Nil(t, claims)
			} else {
				require.NoError(t, err)
				require.Equal(t, "user", claims.Subject)
			}
		})
	}
}

func TestReloadKeepsLastGoodList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "denylist.jsonl")
	denylist := NewDenylist(FileSource{Path: path}, log.NewNoopLogger(), metrics.NoopMetricsHandler, true)
	require.NoError(t, os.WriteFile(path, []byte(`{"jti":"first","exp":9999999999}`+"\n"), 0600))
	denylist.Reload(context.Background())
	require.True(t, denylist.Revoked("first"))
	require.NoError(t, os.WriteFile(path, []byte("bad json\n"), 0600))
	denylist.Reload(context.Background())
	require.True(t, denylist.Revoked("first"))
	require.NoError(t, os.Remove(path))
	denylist.Reload(context.Background())
	require.True(t, denylist.Revoked("first"))
	require.NoError(t, os.WriteFile(path, []byte(`{"jti":"second","exp":9999999999}`+"\n"), 0600))
	denylist.Reload(context.Background())
	require.False(t, denylist.Revoked("first"))
	require.True(t, denylist.Revoked("second"))
}

func TestStartupModes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		failClosed bool
		revoked    bool
	}{
		{"fail closed", true, true},
		{"empty", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			denylist := NewDenylist(FileSource{Path: filepath.Join(t.TempDir(), "missing")}, log.NewNoopLogger(), metrics.NoopMetricsHandler, tc.failClosed)
			denylist.Reload(context.Background())
			require.Equal(t, tc.revoked, denylist.Revoked("key"))
		})
	}
}
