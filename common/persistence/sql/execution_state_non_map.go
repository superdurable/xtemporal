package sql

import (
	"context"
	"database/sql"

	"github.com/superdurable/temporal-server/common/convert"
	p "github.com/superdurable/temporal-server/common/persistence"
	"github.com/superdurable/temporal-server/common/persistence/sql/sqlplugin"
	"github.com/superdurable/temporal-server/common/primitives"
	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/api/serviceerror"
)

func updateSignalsRequested(
	ctx context.Context,
	tx sqlplugin.Tx,
	signalRequestedIDs map[string]struct{},
	deleteIDs map[string]struct{},
	shardID int32,
	namespaceID primitives.UUID,
	workflowID string,
	runID primitives.UUID,
) error {

	if len(signalRequestedIDs) > 0 {
		rows := make([]sqlplugin.SignalsRequestedSetsRow, 0, len(signalRequestedIDs))
		for signalRequestedID := range signalRequestedIDs {
			rows = append(rows, sqlplugin.SignalsRequestedSetsRow{
				NamespaceID: namespaceID,
				WorkflowID:  workflowID,
				RunID:       runID,
				SignalID:    signalRequestedID,
			})
		}
		if _, err := tx.ReplaceIntoSignalsRequestedSets(ctx, rows); err != nil {
			return serviceerror.NewUnavailablef("Failed to update signals requested. Failed to execute update query. Error: %v", err)
		}
	}

	if len(deleteIDs) > 0 {
		if _, err := tx.DeleteFromSignalsRequestedSets(ctx, sqlplugin.SignalsRequestedSetsFilter{
			NamespaceID: namespaceID,
			WorkflowID:  workflowID,
			RunID:       runID,
			SignalIDs:   convert.StringSetToSlice(deleteIDs),
		}); err != nil {
			return serviceerror.NewUnavailablef("Failed to update signals requested. Failed to execute delete query. Error: %v", err)
		}
	}
	return nil
}

func getSignalsRequested(
	ctx context.Context,
	db sqlplugin.DB,
	shardID int32,
	namespaceID primitives.UUID,
	workflowID string,
	runID primitives.UUID,
) ([]string, error) {

	rows, err := db.SelectAllFromSignalsRequestedSets(ctx, sqlplugin.SignalsRequestedSetsAllFilter{
		NamespaceID: namespaceID,
		WorkflowID:  workflowID,
		RunID:       runID,
	})
	if err != nil && err != sql.ErrNoRows {
		return nil, serviceerror.NewUnavailablef("Failed to get signals requested. Error: %v", err)
	}
	var ret = make([]string, len(rows))
	for i, s := range rows {
		ret[i] = s.SignalID
	}
	return ret, nil
}

func deleteSignalsRequestedSet(
	ctx context.Context,
	tx sqlplugin.Tx,
	shardID int32,
	namespaceID primitives.UUID,
	workflowID string,
	runID primitives.UUID,
) error {

	if _, err := tx.DeleteAllFromSignalsRequestedSets(ctx, sqlplugin.SignalsRequestedSetsAllFilter{
		NamespaceID: namespaceID,
		WorkflowID:  workflowID,
		RunID:       runID,
	}); err != nil {
		return serviceerror.NewUnavailablef("Failed to delete signals requested set. Error: %v", err)
	}
	return nil
}

func updateBufferedEvents(
	ctx context.Context,
	tx sqlplugin.Tx,
	batch *commonpb.DataBlob,
	shardID int32,
	namespaceID primitives.UUID,
	workflowID string,
	runID primitives.UUID,
) error {

	if batch == nil {
		return nil
	}
	row := sqlplugin.BufferedEventsRow{
		NamespaceID:  namespaceID,
		WorkflowID:   workflowID,
		RunID:        runID,
		Data:         batch.Data,
		DataEncoding: batch.EncodingType.String(),
	}

	if _, err := tx.InsertIntoBufferedEvents(ctx, []sqlplugin.BufferedEventsRow{row}); err != nil {
		return serviceerror.NewUnavailablef("updateBufferedEvents operation failed. Error: %v", err)
	}
	return nil
}

func getBufferedEvents(
	ctx context.Context,
	db sqlplugin.DB,
	shardID int32,
	namespaceID primitives.UUID,
	workflowID string,
	runID primitives.UUID,
) ([]*commonpb.DataBlob, error) {

	rows, err := db.SelectFromBufferedEvents(ctx, sqlplugin.BufferedEventsFilter{
		NamespaceID: namespaceID,
		WorkflowID:  workflowID,
		RunID:       runID,
	})
	if err != nil && err != sql.ErrNoRows {
		return nil, serviceerror.NewUnavailablef("getBufferedEvents operation failed. Select failed: %v", err)
	}
	var result []*commonpb.DataBlob
	for _, row := range rows {
		result = append(result, p.NewDataBlob(row.Data, row.DataEncoding))
	}
	return result, nil
}

func deleteBufferedEvents(
	ctx context.Context,
	tx sqlplugin.Tx,
	shardID int32,
	namespaceID primitives.UUID,
	workflowID string,
	runID primitives.UUID,
) error {

	if _, err := tx.DeleteFromBufferedEvents(ctx, sqlplugin.BufferedEventsFilter{
		NamespaceID: namespaceID,
		WorkflowID:  workflowID,
		RunID:       runID,
	}); err != nil {
		return serviceerror.NewUnavailablef("updateBufferedEvents delete operation failed. Error: %v", err)
	}
	return nil
}
