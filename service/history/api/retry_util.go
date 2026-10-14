package api

import (
	"go.temporal.io/api/serviceerror"
	"github.com/superdurable/xtemporal/common"
	"github.com/superdurable/xtemporal/common/persistence"
	"github.com/superdurable/xtemporal/common/persistence/versionhistory"
	"github.com/superdurable/xtemporal/service/history/consts"
	historyi "github.com/superdurable/xtemporal/service/history/interfaces"
)

func IsRetryableError(err error) bool {
	return err == consts.ErrStaleState ||
		err == consts.ErrLocateCurrentWorkflowExecution ||
		err == consts.ErrBufferedQueryCleared ||
		persistence.IsConflictErr(err) ||
		common.IsServiceHandlerRetryableError(err)
}

func IsHistoryEventOnCurrentBranch(
	mutableState historyi.MutableState,
	eventID int64,
	eventVersion int64,
) (bool, error) {
	if eventVersion == 0 {
		if eventID >= mutableState.GetNextEventID() {
			return false, &serviceerror.NotFound{Message: "History event not found"}
		}

		// there's no version, assume the event is on the current branch
		return true, nil
	}

	versionHistoryies := mutableState.GetExecutionInfo().GetVersionHistories()
	versionHistoryItem := versionhistory.NewVersionHistoryItem(eventID, eventVersion)
	if _, err := versionhistory.FindFirstVersionHistoryIndexByVersionHistoryItem(
		versionHistoryies,
		versionHistoryItem,
	); err != nil {
		return false, &serviceerror.NotFound{Message: "History event not found"}
	}

	// check if on current branch
	currentVersionHistory, err := versionhistory.GetCurrentVersionHistory(versionHistoryies)
	if err != nil {
		return false, err
	}

	return versionhistory.ContainsVersionHistoryItem(
		currentVersionHistory,
		versionHistoryItem,
	), nil
}
