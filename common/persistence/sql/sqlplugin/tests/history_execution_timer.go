package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/superdurable/temporal-server/common/persistence/sql/sqlplugin"
	"github.com/superdurable/temporal-server/common/primitives"
	"github.com/superdurable/temporal-server/common/shuffle"
)

type (
	historyExecutionTimerSuite struct {
		suite.Suite
		*require.Assertions

		store sqlplugin.HistoryExecutionTimer
	}
)

const (
	testHistoryExecutionTimerID       = "random history timer ID"
	testHistoryExecutionTimerEncoding = "random encoding"
)

var (
	testHistoryExecutionTimerData = []byte("random history execution timer data")
)

func NewHistoryExecutionTimerSuite(
	t *testing.T,
	store sqlplugin.HistoryExecutionTimer,
) *historyExecutionTimerSuite {
	return &historyExecutionTimerSuite{
		Assertions: require.New(t),
		store:      store,
	}
}

func (s *historyExecutionTimerSuite) SetupSuite() {

}

func (s *historyExecutionTimerSuite) TearDownSuite() {

}

func (s *historyExecutionTimerSuite) SetupTest() {
	s.Assertions = require.New(s.T())
}

func (s *historyExecutionTimerSuite) TearDownTest() {

}

func (s *historyExecutionTimerSuite) TestReplace_Single() {
	namespaceID := primitives.NewUUID()
	workflowID := shuffle.String(testHistoryExecutionWorkflowID)
	runID := primitives.NewUUID()
	timerID := shuffle.String(testHistoryExecutionTimerID)

	timer := s.newRandomExecutionTimerRow(namespaceID, workflowID, runID, timerID)
	result, err := s.store.ReplaceIntoTimerInfoMaps(newExecutionContext(), []sqlplugin.TimerInfoMapsRow{timer})
	s.NoError(err)
	rowsAffected, err := result.RowsAffected()
	s.NoError(err)
	s.Equal(1, int(rowsAffected))
}

func (s *historyExecutionTimerSuite) TestReplace_Multiple() {
	namespaceID := primitives.NewUUID()
	workflowID := shuffle.String(testHistoryExecutionWorkflowID)
	runID := primitives.NewUUID()

	timer1 := s.newRandomExecutionTimerRow(namespaceID, workflowID, runID, shuffle.String(testHistoryExecutionTimerID))
	timer2 := s.newRandomExecutionTimerRow(namespaceID, workflowID, runID, shuffle.String(testHistoryExecutionTimerID))
	result, err := s.store.ReplaceIntoTimerInfoMaps(newExecutionContext(), []sqlplugin.TimerInfoMapsRow{timer1, timer2})
	s.NoError(err)
	rowsAffected, err := result.RowsAffected()
	s.NoError(err)
	s.Equal(2, int(rowsAffected))
}

func (s *historyExecutionTimerSuite) TestReplaceSelect_Single() {
	namespaceID := primitives.NewUUID()
	workflowID := shuffle.String(testHistoryExecutionWorkflowID)
	runID := primitives.NewUUID()
	timerID := shuffle.String(testHistoryExecutionTimerID)

	timer := s.newRandomExecutionTimerRow(namespaceID, workflowID, runID, timerID)
	result, err := s.store.ReplaceIntoTimerInfoMaps(newExecutionContext(), []sqlplugin.TimerInfoMapsRow{timer})
	s.NoError(err)
	rowsAffected, err := result.RowsAffected()
	s.NoError(err)
	s.Equal(1, int(rowsAffected))

	selectFilter := sqlplugin.TimerInfoMapsAllFilter{
		NamespaceID: namespaceID,
		WorkflowID:  workflowID,
		RunID:       runID,
	}
	rows, err := s.store.SelectAllFromTimerInfoMaps(newExecutionContext(), selectFilter)
	s.NoError(err)
	rowMap := map[string]sqlplugin.TimerInfoMapsRow{}
	for _, timer := range rows {
		rowMap[timer.TimerID] = timer
	}
	s.Equal(map[string]sqlplugin.TimerInfoMapsRow{
		timer.TimerID: timer,
	}, rowMap)
}

func (s *historyExecutionTimerSuite) TestReplaceSelect_Multiple() {
	numTimers := 20

	namespaceID := primitives.NewUUID()
	workflowID := shuffle.String(testHistoryExecutionWorkflowID)
	runID := primitives.NewUUID()

	var timers []sqlplugin.TimerInfoMapsRow
	for range numTimers {
		timer := s.newRandomExecutionTimerRow(namespaceID, workflowID, runID, shuffle.String(testHistoryExecutionTimerID))
		timers = append(timers, timer)
	}
	result, err := s.store.ReplaceIntoTimerInfoMaps(newExecutionContext(), timers)
	s.NoError(err)
	rowsAffected, err := result.RowsAffected()
	s.NoError(err)
	s.Equal(numTimers, int(rowsAffected))

	selectFilter := sqlplugin.TimerInfoMapsAllFilter{
		NamespaceID: namespaceID,
		WorkflowID:  workflowID,
		RunID:       runID,
	}
	rows, err := s.store.SelectAllFromTimerInfoMaps(newExecutionContext(), selectFilter)
	s.NoError(err)
	timerMap := map[string]sqlplugin.TimerInfoMapsRow{}
	for _, timer := range timers {
		timerMap[timer.TimerID] = timer
	}
	rowMap := map[string]sqlplugin.TimerInfoMapsRow{}
	for _, timer := range rows {
		rowMap[timer.TimerID] = timer
	}
	s.Equal(timerMap, rowMap)
}

func (s *historyExecutionTimerSuite) TestDeleteSelect_Single() {
	namespaceID := primitives.NewUUID()
	workflowID := shuffle.String(testHistoryExecutionWorkflowID)
	runID := primitives.NewUUID()
	timerID := shuffle.String(testHistoryExecutionTimerID)

	deletFilter := sqlplugin.TimerInfoMapsFilter{
		NamespaceID: namespaceID,
		WorkflowID:  workflowID,
		RunID:       runID,
		TimerIDs:    []string{timerID},
	}
	result, err := s.store.DeleteFromTimerInfoMaps(newExecutionContext(), deletFilter)
	s.NoError(err)
	rowsAffected, err := result.RowsAffected()
	s.NoError(err)
	s.Equal(0, int(rowsAffected))

	selectFilter := sqlplugin.TimerInfoMapsAllFilter{
		NamespaceID: namespaceID,
		WorkflowID:  workflowID,
		RunID:       runID,
	}
	rows, err := s.store.SelectAllFromTimerInfoMaps(newExecutionContext(), selectFilter)
	s.NoError(err)
	s.Equal([]sqlplugin.TimerInfoMapsRow(nil), rows)
}

func (s *historyExecutionTimerSuite) TestDeleteSelect_Multiple() {
	namespaceID := primitives.NewUUID()
	workflowID := shuffle.String(testHistoryExecutionWorkflowID)
	runID := primitives.NewUUID()

	deleteFilter := sqlplugin.TimerInfoMapsFilter{
		NamespaceID: namespaceID,
		WorkflowID:  workflowID,
		RunID:       runID,
		TimerIDs:    []string{shuffle.String(testHistoryExecutionTimerID), shuffle.String(testHistoryExecutionTimerID)},
	}
	result, err := s.store.DeleteFromTimerInfoMaps(newExecutionContext(), deleteFilter)
	s.NoError(err)
	rowsAffected, err := result.RowsAffected()
	s.NoError(err)
	s.Equal(0, int(rowsAffected))

	selectFilter := sqlplugin.TimerInfoMapsAllFilter{
		NamespaceID: namespaceID,
		WorkflowID:  workflowID,
		RunID:       runID,
	}
	rows, err := s.store.SelectAllFromTimerInfoMaps(newExecutionContext(), selectFilter)
	s.NoError(err)
	s.Equal([]sqlplugin.TimerInfoMapsRow(nil), rows)
}

func (s *historyExecutionTimerSuite) TestDeleteSelect_All() {
	namespaceID := primitives.NewUUID()
	workflowID := shuffle.String(testHistoryExecutionWorkflowID)
	runID := primitives.NewUUID()

	deleteFilter := sqlplugin.TimerInfoMapsAllFilter{
		NamespaceID: namespaceID,
		WorkflowID:  workflowID,
		RunID:       runID,
	}
	result, err := s.store.DeleteAllFromTimerInfoMaps(newExecutionContext(), deleteFilter)
	s.NoError(err)
	rowsAffected, err := result.RowsAffected()
	s.NoError(err)
	s.Equal(0, int(rowsAffected))

	selectFilter := sqlplugin.TimerInfoMapsAllFilter{
		NamespaceID: namespaceID,
		WorkflowID:  workflowID,
		RunID:       runID,
	}
	rows, err := s.store.SelectAllFromTimerInfoMaps(newExecutionContext(), selectFilter)
	s.NoError(err)
	s.Equal([]sqlplugin.TimerInfoMapsRow(nil), rows)
}

func (s *historyExecutionTimerSuite) TestReplaceDeleteSelect_Single() {
	namespaceID := primitives.NewUUID()
	workflowID := shuffle.String(testHistoryExecutionWorkflowID)
	runID := primitives.NewUUID()
	timerID := shuffle.String(testHistoryExecutionTimerID)

	timer := s.newRandomExecutionTimerRow(namespaceID, workflowID, runID, timerID)
	result, err := s.store.ReplaceIntoTimerInfoMaps(newExecutionContext(), []sqlplugin.TimerInfoMapsRow{timer})
	s.NoError(err)
	rowsAffected, err := result.RowsAffected()
	s.NoError(err)
	s.Equal(1, int(rowsAffected))

	deleteFilter := sqlplugin.TimerInfoMapsFilter{
		NamespaceID: namespaceID,
		WorkflowID:  workflowID,
		RunID:       runID,
		TimerIDs:    []string{timerID},
	}
	result, err = s.store.DeleteFromTimerInfoMaps(newExecutionContext(), deleteFilter)
	s.NoError(err)
	rowsAffected, err = result.RowsAffected()
	s.NoError(err)
	s.Equal(1, int(rowsAffected))

	selectFilter := sqlplugin.TimerInfoMapsAllFilter{
		NamespaceID: namespaceID,
		WorkflowID:  workflowID,
		RunID:       runID,
	}
	rows, err := s.store.SelectAllFromTimerInfoMaps(newExecutionContext(), selectFilter)
	s.NoError(err)
	s.Equal([]sqlplugin.TimerInfoMapsRow(nil), rows)
}

func (s *historyExecutionTimerSuite) TestReplaceDeleteSelect_Multiple() {
	numTimers := 20

	namespaceID := primitives.NewUUID()
	workflowID := shuffle.String(testHistoryExecutionWorkflowID)
	runID := primitives.NewUUID()

	var timers []sqlplugin.TimerInfoMapsRow
	var timerIDs []string
	for range numTimers {
		timerID := shuffle.String(testHistoryExecutionTimerID)
		timer := s.newRandomExecutionTimerRow(namespaceID, workflowID, runID, timerID)
		timerIDs = append(timerIDs, timerID)
		timers = append(timers, timer)
	}
	result, err := s.store.ReplaceIntoTimerInfoMaps(newExecutionContext(), timers)
	s.NoError(err)
	rowsAffected, err := result.RowsAffected()
	s.NoError(err)
	s.Equal(numTimers, int(rowsAffected))

	deleteFilter := sqlplugin.TimerInfoMapsFilter{
		NamespaceID: namespaceID,
		WorkflowID:  workflowID,
		RunID:       runID,
		TimerIDs:    timerIDs,
	}
	result, err = s.store.DeleteFromTimerInfoMaps(newExecutionContext(), deleteFilter)
	s.NoError(err)
	rowsAffected, err = result.RowsAffected()
	s.NoError(err)
	s.Equal(numTimers, int(rowsAffected))

	selectFilter := sqlplugin.TimerInfoMapsAllFilter{
		NamespaceID: namespaceID,
		WorkflowID:  workflowID,
		RunID:       runID,
	}
	rows, err := s.store.SelectAllFromTimerInfoMaps(newExecutionContext(), selectFilter)
	s.NoError(err)
	s.Equal([]sqlplugin.TimerInfoMapsRow(nil), rows)
}

func (s *historyExecutionTimerSuite) TestReplaceDeleteSelect_All() {
	numTimers := 20

	namespaceID := primitives.NewUUID()
	workflowID := shuffle.String(testHistoryExecutionWorkflowID)
	runID := primitives.NewUUID()

	var timers []sqlplugin.TimerInfoMapsRow
	for range numTimers {
		timer := s.newRandomExecutionTimerRow(namespaceID, workflowID, runID, shuffle.String(testHistoryExecutionTimerID))
		timers = append(timers, timer)
	}
	result, err := s.store.ReplaceIntoTimerInfoMaps(newExecutionContext(), timers)
	s.NoError(err)
	rowsAffected, err := result.RowsAffected()
	s.NoError(err)
	s.Equal(numTimers, int(rowsAffected))

	deleteFilter := sqlplugin.TimerInfoMapsAllFilter{
		NamespaceID: namespaceID,
		WorkflowID:  workflowID,
		RunID:       runID,
	}
	result, err = s.store.DeleteAllFromTimerInfoMaps(newExecutionContext(), deleteFilter)
	s.NoError(err)
	rowsAffected, err = result.RowsAffected()
	s.NoError(err)
	s.Equal(numTimers, int(rowsAffected))

	selectFilter := sqlplugin.TimerInfoMapsAllFilter{
		NamespaceID: namespaceID,
		WorkflowID:  workflowID,
		RunID:       runID,
	}
	rows, err := s.store.SelectAllFromTimerInfoMaps(newExecutionContext(), selectFilter)
	s.NoError(err)
	s.Equal([]sqlplugin.TimerInfoMapsRow(nil), rows)
}

func (s *historyExecutionTimerSuite) newRandomExecutionTimerRow(
	namespaceID primitives.UUID,
	workflowID string,
	runID primitives.UUID,
	timerID string,
) sqlplugin.TimerInfoMapsRow {
	return sqlplugin.TimerInfoMapsRow{
		NamespaceID:  namespaceID,
		WorkflowID:   workflowID,
		RunID:        runID,
		TimerID:      timerID,
		Data:         shuffle.Bytes(testHistoryExecutionTimerData),
		DataEncoding: testHistoryExecutionTimerEncoding,
	}
}
