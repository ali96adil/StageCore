package assistant

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrProviderUnavailable = errors.New("assistant provider unavailable")
	ErrFakeResponseMissing = errors.New("deterministic fake response missing")
)

// Provider is the replaceable model boundary. StageCore product semantics and
// authorization remain outside the provider implementation.
type Provider interface {
	Complete(context.Context, Request) (Response, error)
}

// FakeProvider is deterministic test infrastructure. It never performs network
// access and returns only the response explicitly registered for a request ID.
type FakeProvider struct {
	Responses map[string]Response
	Errors    map[string]error
}

func (f FakeProvider) Complete(ctx context.Context, request Request) (Response, error) {
	if err := request.Validate(); err != nil {
		return Response{}, err
	}
	select {
	case <-ctx.Done():
		return Response{}, ctx.Err()
	default:
	}
	if err, ok := f.Errors[request.RequestID]; ok && err != nil {
		return Response{}, err
	}
	response, ok := f.Responses[request.RequestID]
	if !ok {
		return Response{}, fmt.Errorf("%w: %s", ErrFakeResponseMissing, request.RequestID)
	}
	if err := response.ValidateFor(request); err != nil {
		return Response{}, err
	}
	return response, nil
}
