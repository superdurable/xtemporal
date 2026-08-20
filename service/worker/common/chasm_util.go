package common

import (
	"fmt"
	"strconv"
	"unicode"

	"github.com/superdurable/temporal-server/chasm"
	"github.com/superdurable/temporal-server/common/payload"
	"github.com/superdurable/temporal-server/common/searchattribute/sadefs"
	workflowpb "go.temporal.io/api/workflow/v1"
)

func ArchetypeIDFromExecutionInfo(
	executionInfo *workflowpb.WorkflowExecutionInfo,
) (chasm.ArchetypeID, error) {
	indexedField := executionInfo.SearchAttributes.GetIndexedFields()
	if indexedField == nil {
		return chasm.WorkflowArchetypeID, nil
	}

	nsDivisionPayload, ok := indexedField[sadefs.TemporalNamespaceDivision]
	if !ok {
		return chasm.WorkflowArchetypeID, nil
	}

	var nsDivisionStr string
	if err := payload.Decode(nsDivisionPayload, &nsDivisionStr); err != nil {
		return chasm.UnspecifiedArchetypeID, fmt.Errorf("failed to decode TemporalNamespaceDivision field: %w", err)
	}

	if len(nsDivisionStr) == 0 || !unicode.IsDigit(rune(nsDivisionStr[0])) {
		return chasm.WorkflowArchetypeID, nil
	}

	archetypeID, err := strconv.ParseUint(nsDivisionStr, 10, 32)
	if err != nil {
		return chasm.UnspecifiedArchetypeID, fmt.Errorf("failed to parse archetypeID: %w", err)
	}

	return chasm.ArchetypeID(archetypeID), nil
}
