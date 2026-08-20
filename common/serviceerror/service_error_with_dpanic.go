package serviceerror

import (
	"github.com/superdurable/temporal-server/common/log"
	"go.temporal.io/api/serviceerror"
)

// NewInternalErrorWithDPanic is a wrapper for service error that will panic if it's in dev environment
func NewInternalErrorWithDPanic(logger log.Logger, msg string) error {
	logger.DPanic(msg)
	return serviceerror.NewInternal(msg)
}
