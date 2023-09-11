package plnexectr

import (
	"context"
	"fmt"
	"time"

	"github.com/dominikbraun/graph"
	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	"helm.sh/helm/v3/pkg/werf/log"
	"helm.sh/helm/v3/pkg/werf/opertn"
	"helm.sh/helm/v3/pkg/werf/pln"
)

func NewPlanExecutor(plan *pln.Plan, opts PlanExecutorOptions) *PlanExecutor {
	return &PlanExecutor{
		plan:               plan,
		networkParallelism: lo.Max([]int{opts.NetworkParallelism, 1}),
	}
}

type PlanExecutorOptions struct {
	NetworkParallelism int
}

type PlanExecutor struct {
	plan               *pln.Plan
	networkParallelism int
}

func (e *PlanExecutor) Execute(parentCtx context.Context) error {
	ctx, ctxCancelFn := context.WithCancel(parentCtx)

	opsMap, err := e.plan.PredecessorMap()
	if err != nil {
		return fmt.Errorf("error getting plan predecessor map: %w", err)
	}

	workerPool := pool.New().WithContext(ctx).WithMaxGoroutines(e.networkParallelism)
	completedOpsIDsCh := make(chan string, 100000)

	for i := 0; len(opsMap) > 0; i++ {
		if i == 0 {
			executableOpsIDs := e.findExecutableOpsIDs(opsMap)
			for _, opID := range executableOpsIDs {
				opID := opID
				delete(opsMap, opID)
				e.execOperation(opID, completedOpsIDsCh, workerPool, ctxCancelFn)
			}
			continue
		}

		if ctx.Err() != nil {
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

		executableOpsIDs := e.findExecutableOpsIDs(opsMap)
		for _, opID := range executableOpsIDs {
			opID := opID
			delete(opsMap, opID)
			e.execOperation(opID, completedOpsIDsCh, workerPool, ctxCancelFn)
		}
	}

	if err := workerPool.Wait(); err != nil {
		return fmt.Errorf("error waiting for operations completion: %w", err)
	}

	return nil
}

func (e *PlanExecutor) execOperation(opID string, completedOpsIDsCh chan string, workerPool *pool.ContextPool, ctxCancelFn context.CancelFunc) {
	workerPool.Go(func(ctx context.Context) error {
		op := lo.Must(e.plan.Operation(opID))

		switch op.Type() {
		case opertn.TypeCreateResourceOperation,
			opertn.TypeRecreateResourceOperation,
			opertn.TypeApplyResourceOperation,
			opertn.TypeDeleteResourceOperation:
			log.Default.Info(ctx, "Executing operation %q ...", op.HumanID())
		}
		if err := op.Execute(ctx); err != nil {
			ctxCancelFn()
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
