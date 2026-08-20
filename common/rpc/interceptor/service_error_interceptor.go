package interceptor

import (
	"context"
	"errors"

	"github.com/superdurable/temporal-server/common/persistence/serialization"
	"go.temporal.io/api/serviceerror"
	"google.golang.org/grpc"
)

func ServiceErrorInterceptor(
	ctx context.Context,
	req any,
	_ *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {

	resp, err := handler(ctx, req)

	var deserializationError *serialization.DeserializationError
	var serializationError *serialization.SerializationError
	// convert serialization errors to be captured as serviceerrors across gRPC calls
	if errors.As(err, &deserializationError) || errors.As(err, &serializationError) {
		err = serviceerror.NewDataLoss(err.Error())
	}
	return resp, serviceerror.ToStatus(err).Err()
}
