package background

import (
	"context"

	"github.com/gmhelper/notify-api/internal/app/email"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type Processor struct {
	sender email.Sender
	logger logger.Logger
}

func NewProcessor(sender email.Sender, logger logger.Logger) *Processor {
	return &Processor{sender: sender, logger: logger}
}

func (p *Processor) Start(ctx context.Context) {
	p.logger.Info("background processor initialized")
}
