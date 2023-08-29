package work

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/response"
	"github.com/streamingfast/substreams/orchestrator/stage"
)

func Test_workerPoolPool_Borrow_Return(t *testing.T) {
	ctx := context.Background()
	pi := NewWorkerPool(ctx, 2, 2, 0, func(logger *zap.Logger) Worker {
		return NewWorkerFactoryFromFunc(func(ctx context.Context, unit stage.Unit, workRange *block.Range, moduleNames []string, upstream *response.Stream) loop.Cmd {
			return func() loop.Msg {
				return &Result{}
			}
		})
	})

	assert.Len(t, pi.workers, 2)
	assert.True(t, pi.WorkerAvailable())
	worker1 := pi.Borrow()
	assert.True(t, pi.WorkerAvailable())
	worker2 := pi.Borrow()
	assert.False(t, pi.WorkerAvailable())
	assert.Panics(t, func() { pi.Borrow() })
	pi.Return(worker2)
	assert.True(t, pi.WorkerAvailable())
	pi.Return(worker1)
	assert.Panics(t, func() { pi.Return(worker1) })
}
