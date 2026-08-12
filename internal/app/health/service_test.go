package health

import (
	"context"
	"errors"
	"testing"
)

type fakePinger struct {
	err error
}

func (f *fakePinger) Ping(ctx context.Context) error {
	return f.err
}

func TestReadinessService_Check(t *testing.T) {
	service := NewReadinessService(&fakePinger{})
	if err := service.Check(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestReadinessService_CheckFailure(t *testing.T) {
	errSentinel := errors.New("db offline")
	service := NewReadinessService(&fakePinger{err: errSentinel})
	if err := service.Check(context.Background()); !errors.Is(err, errSentinel) {
		t.Fatalf("expected %v, got %v", errSentinel, err)
	}
}
