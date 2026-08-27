package persistence

import (
	"testing"

	"github.com/stretchr/testify/require"
	persistencespb "github.com/superdurable/temporal-server/api/persistence/v1"
)

func TestValidateHistoryShardCountTransition(t *testing.T) {
	testCases := []struct {
		name    string
		old     int32
		cur     int32
		wantErr bool
	}{
		{name: "unchanged", old: 512, cur: 512},
		{name: "doubled", old: 512, cur: 1024},
		{name: "quadrupled", old: 512, cur: 2048},
		{name: "tripled", old: 4, cur: 12},
		{name: "from one", old: 1, cur: 16},
		{name: "decrease", old: 1024, cur: 512, wantErr: true},
		{name: "non-multiple increase", old: 512, cur: 768, wantErr: true},
		{name: "off by one", old: 512, cur: 513, wantErr: true},
		{name: "zero old", old: 0, cur: 512, wantErr: true},
		{name: "zero cur", old: 512, cur: 0, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateHistoryShardCountTransition(tc.old, tc.cur)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestImmutableFieldsChanged_HistoryShardCount(t *testing.T) {
	// The shard count is no longer strictly immutable: a legal increase must be allowed through so
	// that a deliberate resharding can be persisted, while anything else stays rejected.
	testCases := []struct {
		name string
		old  int32
		cur  int32
		want bool
	}{
		{name: "unchanged", old: 512, cur: 512, want: false},
		{name: "legal increase", old: 512, cur: 1024, want: false},
		{name: "unset old is filled in", old: 0, cur: 512, want: false},
		{name: "decrease", old: 1024, cur: 512, want: true},
		{name: "non-multiple", old: 512, cur: 768, want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			old := &persistencespb.ClusterMetadata{HistoryShardCount: tc.old}
			cur := &persistencespb.ClusterMetadata{HistoryShardCount: tc.cur}
			require.Equal(t, tc.want, immutableFieldsChanged(old, cur))
		})
	}
}
