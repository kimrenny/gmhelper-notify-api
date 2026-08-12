package health

import "context"

type Pinger interface {
	Ping(ctx context.Context) error
}

type ReadinessService struct {
	pinger Pinger
}

func NewReadinessService(pinger Pinger) *ReadinessService {
	return &ReadinessService{pinger: pinger}
}

func (s *ReadinessService) Check(ctx context.Context) error {
	return s.pinger.Ping(ctx)
}
