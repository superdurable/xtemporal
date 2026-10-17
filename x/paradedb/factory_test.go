package paradedb

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/searchattribute"
	"go.temporal.io/api/serviceerror"
)

func TestParseStoreOptions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options map[string]any
		wantErr bool
	}{
		{name: "valid", options: map[string]any{"profile": "dex", "sql": map[string]any{
			"pluginName": "postgres12_pgx", "connectAddr": "localhost:5432", "databaseName": "visibility",
			"user": "temporal", "password": "secret", "maxConns": 12,
			"connectAttributes": map[string]string{"application_name": "dex"},
		}}},
		{name: "unknown top level", options: map[string]any{"other": true}, wantErr: true},
		{name: "unknown sql", options: map[string]any{"sql": map[string]any{"connectAddr": "x", "databaseName": "d", "other": true}}, wantErr: true},
		{name: "invalid profile", options: map[string]any{"profile": "cloud", "sql": map[string]any{"connectAddr": "x", "databaseName": "d"}}, wantErr: true},
		{name: "invalid plugin", options: map[string]any{"sql": map[string]any{"pluginName": "mysql", "connectAddr": "x", "databaseName": "d"}}, wantErr: true},
		{name: "missing database", options: map[string]any{"sql": map[string]any{"connectAddr": "x"}}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseStoreOptions(tc.options)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, "postgres12_pgx", got.SQL.PluginName)
				require.Equal(t, 12, got.SQL.MaxConns)
			}
		})
	}
}

type failingProvider struct{}

func (failingProvider) GetSearchAttributes(string, bool) (searchattribute.NameTypeMap, error) {
	return searchattribute.NameTypeMap{}, errors.New("registry unavailable")
}

func TestProviderFailure(t *testing.T) {
	s := &VisibilityStore{indexName: "test", saProvider: failingProvider{}}
	_, err := s.ValidateCustomSearchAttributes(map[string]any{"FlowType": "a"})
	require.ErrorAs(t, err, new(*serviceerror.Unavailable))
	_, err = s.encodeRequestAttributes(nil)
	require.ErrorAs(t, err, new(*serviceerror.Unavailable))
}
