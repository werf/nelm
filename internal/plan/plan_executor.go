package plan

import (
	"context"
	"fmt"
	"time"

	"github.com/davecgh/go-spew/spew"
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
	log.Default.Debug(parentCtx, "LESIKOVTEST: start Execute()")
	ctx, ctxCancelFn := context.WithCancel(parentCtx)

	opsMap, err := e.plan.PredecessorMap()
	if err != nil {
		return fmt.Errorf("error getting plan predecessor map: %w", err)
	}
	log.Default.Debug(ctx, "LESIKOVTEST: opsMap: %s", spew.Sdump(opsMap))

	workerPool := pool.New().WithContext(ctx).WithMaxGoroutines(e.networkParallelism).WithCancelOnError().WithFirstError()
	completedOpsIDsCh := make(chan string, 100000)

	log.Default.Debug(ctx, "Starting plan operations")
	for i := 0; len(opsMap) > 0; i++ {
		log.Default.Debug(ctx, "LESIKOVTEST: iteration %d", i)
		if i > 0 {
			if ctx.Err() != nil {
				log.Default.Debug(ctx, "LESIKOVTEST: breaking because ctx.Err() is not nil: %v", ctx.Err())
				break
			}

			var gotCompletedOpID bool
			for len(completedOpsIDsCh) > 0 {
				log.Default.Debug(ctx, "LESIKOVTEST: processing completedOpsIDsCh")
				completedOpID := <-completedOpsIDsCh
				log.Default.Debug(ctx, "LESIKOVTEST: completedOpID: %s", completedOpID)
				gotCompletedOpID = true
				for _, edgeMap := range opsMap {
					log.Default.Debug(ctx, "LESIKOVTEST: deleting completedOpID %s from edgeMap", completedOpID)
					delete(edgeMap, completedOpID)
				}
			}
			if !gotCompletedOpID {
				log.Default.Debug(ctx, "LESIKOVTEST: no completedOpID received, sleeping for 100ms")
				time.Sleep(100 * time.Millisecond)
				continue
			}
			log.Default.Debug(ctx, "LESIKOVTEST: got completedOpID, continuing to next iteration")
		}

		executableOpsIDs := e.findExecutableOpsIDs(opsMap)
		log.Default.Debug(ctx, "LESIKOVTEST: executableOpsIDs: %v", spew.Sdump(executableOpsIDs))
		for _, opID := range executableOpsIDs {
			log.Default.Debug(ctx, "LESIKOVTEST: opID: %s", opID)
			opID := opID
			delete(opsMap, opID)
			log.Default.Debug(ctx, "LESIKOVTEST: executing operation %s", opID)
			e.execOperation(ctx, opID, completedOpsIDsCh, workerPool, ctxCancelFn)
		}
	}

	log.Default.Debug(ctx, "Waiting for all plan operations to complete")
	if err := workerPool.Wait(); err != nil {
		log.Default.Debug(ctx, "Error waiting for operations completion: %v", err)
		return fmt.Errorf("error waiting for operations completion: %w", err)
	}

	log.Default.Debug(ctx, "All plan operations completed successfully")
	return nil
}

func (e *PlanExecutor) execOperation(ctx context.Context, opID string, completedOpsIDsCh chan string, workerPool *pool.ContextPool, ctxCancelFn context.CancelFunc) {
	log.Default.Debug(ctx, "LESIKOVTEST: starting goroutine for operation %s", opID)
	workerPool.Go(func(ctx context.Context) error {
		log.Default.Debug(ctx, "LESIKOVTEST: inside goroutine for operation %s", opID)
		failed := true
		defer func() {
			log.Default.Debug(ctx, "LESIKOVTEST: deferred function for operation %s", opID)
			if failed {
				log.Default.Debug(ctx, "LESIKOVTEST: operation %s failed, cancelling context", opID)
				ctxCancelFn()
			}
			log.Default.Debug(ctx, "LESIKOVTEST: deferred function for operation %s completed", opID)
		}()

		log.Default.Debug(ctx, "LESIKOVTEST: getting operation %s from plan", opID)
		op := lo.Must(e.plan.Operation(opID))
		log.Default.Debug(ctx, "LESIKOVTEST: got operation %s from plan", opID)

		log.Default.Debug(ctx, util.Capitalize(op.HumanID()))
		if err := op.Execute(ctx); err != nil {
			log.Default.Debug(ctx, "Error executing operation %s: %v", op.HumanID(), err)
			return fmt.Errorf("error executing operation: %w", err)
		}

		log.Default.Debug(ctx, "LESIKOVTEST: operation %s completed successfully", opID)
		completedOpsIDsCh <- opID

		failed = false
		log.Default.Debug(ctx, "LESIKOVTEST: operation %s completed, returning nil", opID)
		return nil
	})
	log.Default.Debug(ctx, "LESIKOVTEST: goroutine for operation %s started", opID)
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
