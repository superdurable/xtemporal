package limitless

import (
	"context"
	"database/sql"

	"github.com/superdurable/xtemporal/common/persistence/sql/sqlplugin"
)

type tx struct{ sqlplugin.Tx }

var _ sqlplugin.Tx = (*tx)(nil)

func (t *tx) InsertIntoBufferedEvents(ctx context.Context, rows []sqlplugin.BufferedEventsRow) (sql.Result, error) {
	return insertIntoBufferedEvents(ctx, t.Tx, rows)
}
