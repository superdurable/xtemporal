package api

import (
	"fmt"

	"github.com/superdurable/temporal-server/service/history/tasks"
	"go.temporal.io/api/serviceerror"
)

func GetTaskCategory(categoryID int, registry tasks.TaskCategoryRegistry) (tasks.Category, error) {
	category, ok := registry.GetCategoryByID(categoryID)
	if !ok {
		return tasks.Category{}, serviceerror.NewInvalidArgument(
			fmt.Sprintf("Invalid task category id: %v", categoryID),
		)
	}
	return category, nil
}
