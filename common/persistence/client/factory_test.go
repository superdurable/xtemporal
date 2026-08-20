package client_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superdurable/temporal-server/common/config"
	"github.com/superdurable/temporal-server/common/persistence/client"
	"github.com/superdurable/temporal-server/common/persistence/mock"
	"go.uber.org/mock/gomock"
)

func TestFactoryImpl_NewHistoryTaskQueueManager(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
	}{
		{
			name: "No error",
			err:  nil,
		},
		{
			name: "Error",
			err:  errors.New("some error"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			dataStoreFactory := mock.NewMockDataStoreFactory(ctrl)
			dataStoreFactory.EXPECT().NewQueueV2().Return(nil, tc.err).AnyTimes()

			factory := client.NewFactory(
				dataStoreFactory,
				&config.Persistence{
					NumHistoryShards: 1,
				},
				nil,
				nil,
				nil,
				nil,
				nil,
				"",
				nil,
				nil,
				nil,
				func() bool { return false },
				func() bool { return false },
			)
			historyTaskQueueManager, err := factory.NewHistoryTaskQueueManager()
			if tc.err != nil {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, historyTaskQueueManager)
		})
	}
}
