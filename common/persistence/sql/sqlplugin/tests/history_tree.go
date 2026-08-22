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
	historyTreeSuite struct {
		suite.Suite
		*require.Assertions

		store sqlplugin.HistoryTree
	}
)

const (
	testHistoryTreeEncoding = "random encoding"
)

var (
	testHistoryTreeData = []byte("random history tree data")
)

func NewHistoryTreeSuite(
	t *testing.T,
	store sqlplugin.HistoryTree,
) *historyTreeSuite {
	return &historyTreeSuite{
		Assertions: require.New(t),
		store:      store,
	}
}

func (s *historyTreeSuite) SetupSuite() {

}

func (s *historyTreeSuite) TearDownSuite() {

}

func (s *historyTreeSuite) SetupTest() {
	s.Assertions = require.New(s.T())
}

func (s *historyTreeSuite) TearDownTest() {

}

func (s *historyTreeSuite) TestInsert_Success() {
	treeID := primitives.NewUUID()
	branchID := primitives.NewUUID()

	node := s.newRandomTreeRow(treeID, branchID)
	result, err := s.store.InsertIntoHistoryTree(newExecutionContext(), &node)
	s.NoError(err)
	rowsAffected, err := result.RowsAffected()
	s.NoError(err)
	s.Equal(1, int(rowsAffected))
}

func (s *historyTreeSuite) TestInsert_Duplicate_Success() {
	treeID := primitives.NewUUID()
	branchID := primitives.NewUUID()

	node := s.newRandomTreeRow(treeID, branchID)
	result, err := s.store.InsertIntoHistoryTree(newExecutionContext(), &node)
	s.NoError(err)
	rowsAffected, err := result.RowsAffected()
	s.NoError(err)
	s.Equal(1, int(rowsAffected))

	node = s.newRandomTreeRow(treeID, branchID)
	result, err = s.store.InsertIntoHistoryTree(newExecutionContext(), &node)
	s.NoError(err)
	_, err = result.RowsAffected()
	s.NoError(err)
	// TODO cannot assert on the number of rows affect
	//  since MySQL and PostgreSQL have different behavior
}

func (s *historyTreeSuite) TestInsertSelect() {
	treeID := primitives.NewUUID()
	branchID := primitives.NewUUID()

	tree := s.newRandomTreeRow(treeID, branchID)
	result, err := s.store.InsertIntoHistoryTree(newExecutionContext(), &tree)
	s.NoError(err)
	rowsAffected, err := result.RowsAffected()
	s.NoError(err)
	s.Equal(1, int(rowsAffected))

	selectFilter := sqlplugin.HistoryTreeSelectFilter{
		TreeID: treeID,
	}
	rows, err := s.store.SelectFromHistoryTree(newExecutionContext(), selectFilter)
	s.NoError(err)
	for index := range rows {
		rows[index].TreeID = treeID
	}
	s.Equal([]sqlplugin.HistoryTreeRow{tree}, rows)
}

func (s *historyTreeSuite) TestDeleteSelect() {
	treeID := primitives.NewUUID()
	branchID := primitives.NewUUID()

	deleteFilter := sqlplugin.HistoryTreeDeleteFilter{
		TreeID:   treeID,
		BranchID: branchID,
	}
	result, err := s.store.DeleteFromHistoryTree(newExecutionContext(), deleteFilter)
	s.NoError(err)
	rowsAffected, err := result.RowsAffected()
	s.NoError(err)
	s.Equal(0, int(rowsAffected))

	selectFilter := sqlplugin.HistoryTreeSelectFilter{
		TreeID: treeID,
	}
	rows, err := s.store.SelectFromHistoryTree(newExecutionContext(), selectFilter)
	s.NoError(err)
	for index := range rows {
		rows[index].TreeID = treeID
	}
	s.Equal([]sqlplugin.HistoryTreeRow(nil), rows)
}

func (s *historyTreeSuite) TestInsertDeleteSelect() {
	treeID := primitives.NewUUID()
	branchID := primitives.NewUUID()

	tree := s.newRandomTreeRow(treeID, branchID)
	result, err := s.store.InsertIntoHistoryTree(newExecutionContext(), &tree)
	s.NoError(err)
	rowsAffected, err := result.RowsAffected()
	s.NoError(err)
	s.Equal(1, int(rowsAffected))

	deleteFilter := sqlplugin.HistoryTreeDeleteFilter{
		TreeID:   treeID,
		BranchID: branchID,
	}
	result, err = s.store.DeleteFromHistoryTree(newExecutionContext(), deleteFilter)
	s.NoError(err)
	rowsAffected, err = result.RowsAffected()
	s.NoError(err)
	s.Equal(1, int(rowsAffected))

	selectFilter := sqlplugin.HistoryTreeSelectFilter{
		TreeID: treeID,
	}
	rows, err := s.store.SelectFromHistoryTree(newExecutionContext(), selectFilter)
	s.NoError(err)
	for index := range rows {
		rows[index].TreeID = treeID
	}
	s.Equal([]sqlplugin.HistoryTreeRow(nil), rows)
}

func (s *historyTreeSuite) newRandomTreeRow(
	treeID primitives.UUID,
	branchID primitives.UUID,
) sqlplugin.HistoryTreeRow {
	return sqlplugin.HistoryTreeRow{
		TreeID:       treeID,
		BranchID:     branchID,
		Data:         shuffle.Bytes(testHistoryTreeData),
		DataEncoding: testHistoryTreeEncoding,
	}
}
