package plan

import (
	"context"
	"fmt"
	"time"

	"github.com/dominikbraun/graph"
	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"

	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/log"
)

func NewPlanExecutor(plan *Plan, opts PlanExecutorOptions) *PlanExecutor {
	return &PlanExecutor{
		plan:               plan,
		networkParallelism: lo.Max([]int{opts.NetworkParallelism, 1}),
	}
}

type PlanExecutorOptions struct {
	NetworkParallelism int
}

type PlanExecutor struct {
	plan               *Plan
	networkParallelism int
}

func (e *PlanExecutor) Execute(parentCtx context.Context) error {
	ctx, ctxCancelFn := context.WithCancelCause(parentCtx)
	defer ctxCancelFn(fmt.Errorf("context canceled: plan execution finished"))

	opsMap, err := e.plan.PredecessorMap()
	if err != nil {
		return fmt.Errorf("error getting plan predecessor map: %w", err)
	}

	workerPool := pool.New().WithContext(ctx).WithMaxGoroutines(e.networkParallelism).WithCancelOnError().WithFirstError()
	completedOpsIDsCh := make(chan string, 100000)

	log.Default.Debug(ctx, "Starting plan operations")
	for i := 0; len(opsMap) > 0; i++ {
		if i > 0 {
			if ctx.Err() != nil {
				log.Default.Debug(ctx, "Stop scheduling plan operations due to context canceled: %s", context.Cause(ctx))

				break
			}

			var gotCompletedOpID bool
			for len(completedOpsIDsCh) > 0 {
				completedOpID := <-completedOpsIDsCh
				gotCompletedOpID = true
				for _, edgeMap := range opsMap {
					delete(edgeMap, completedOpID)
				}
			}
			if !gotCompletedOpID {
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}

		executableOpsIDs := e.findExecutableOpsIDs(opsMap)
		for _, opID := range executableOpsIDs {
			opID := opID
			delete(opsMap, opID)
			e.execOperation(opID, completedOpsIDsCh, workerPool, ctxCancelFn)
		}
	}

	log.Default.Debug(ctx, "Waiting for all plan operations to complete")
	if err := workerPool.Wait(); err != nil {
		return fmt.Errorf("error waiting for operations completion: %w", err)
	}

	return nil
}

func (e *PlanExecutor) execOperation(opID string, completedOpsIDsCh chan string, workerPool *pool.ContextPool, ctxCancelFn context.CancelCauseFunc) {
	workerPool.Go(func(ctx context.Context) error {
		var err error
		defer func() {
			if err != nil {
				ctxCancelFn(fmt.Errorf("context canceled: %w", err))
			}
		}()

		op := lo.Must(e.plan.Operation(opID))

		log.Default.Debug(ctx, util.Capitalize(op.HumanID()))
		err = op.Execute(ctx)
		if err != nil {
			return fmt.Errorf("error executing operation: %w", err)
		}

		completedOpsIDsCh <- opID

		return nil
	})
}

func (e *PlanExecutor) findExecutableOpsIDs(opsMap map[string]map[string]graph.Edge[string]) []string {
	var executableOpsIDs []string
	for opID, edgeMap := range opsMap {
		if len(edgeMap) == 0 {
			executableOpsIDs = append(executableOpsIDs, opID)
		}
	}

	return executableOpsIDs
}
