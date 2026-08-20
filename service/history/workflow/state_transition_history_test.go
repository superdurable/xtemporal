package workflow_test

import (
	"testing"

	persistencespb "github.com/superdurable/temporal-server/api/persistence/v1"
	"github.com/superdurable/temporal-server/common/testing/protorequire"
	"github.com/superdurable/temporal-server/service/history/workflow"
)

func TestUpdatedTranstionHistory(t *testing.T) {
	var hist []*persistencespb.VersionedTransition
	hist = workflow.UpdatedTransitionHistory(hist, 1)
	protorequire.ProtoSliceEqual(t,
		[]*persistencespb.VersionedTransition{{NamespaceFailoverVersion: 1, TransitionCount: 1}},
		hist,
	)
	hist = workflow.UpdatedTransitionHistory(hist, 1)
	protorequire.ProtoSliceEqual(t,
		[]*persistencespb.VersionedTransition{{NamespaceFailoverVersion: 1, TransitionCount: 2}},
		hist,
	)
	hist = workflow.UpdatedTransitionHistory(hist, 2)
	protorequire.ProtoSliceEqual(t,
		[]*persistencespb.VersionedTransition{
			{NamespaceFailoverVersion: 1, TransitionCount: 2},
			{NamespaceFailoverVersion: 2, TransitionCount: 3},
		},
		hist,
	)
	hist = workflow.UpdatedTransitionHistory(hist, 2)
	protorequire.ProtoSliceEqual(t,
		[]*persistencespb.VersionedTransition{
			{NamespaceFailoverVersion: 1, TransitionCount: 2},
			{NamespaceFailoverVersion: 2, TransitionCount: 4},
		},
		hist,
	)
}
