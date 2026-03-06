package vetu

import (
	"context"

	"github.com/cirruslabs/orchard/internal/worker/vmmanager"
	"github.com/cirruslabs/orchard/internal/worker/vmmanager/base"
	"go.uber.org/zap"
)

const vetuCommandName = "vetu"

func Vetu(ctx context.Context, logger *zap.SugaredLogger, args ...string) (string, string, error) {
	return base.Cmd(ctx, logger, vetuCommandName, args...)
}

func List(ctx context.Context, logger *zap.SugaredLogger) ([]vmmanager.VMInfo, error) {
	return base.List(ctx, logger, vetuCommandName)
}
