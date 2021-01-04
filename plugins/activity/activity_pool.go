package activity

import (
	"context"
	jsoniter "github.com/json-iterator/go"
	"github.com/spiral/errors"
	"github.com/spiral/roadrunner/v2/interfaces/events"
	"github.com/spiral/roadrunner/v2/interfaces/pool"
	rrWorker "github.com/spiral/roadrunner/v2/interfaces/worker"
	poolImpl "github.com/spiral/roadrunner/v2/pkg/pool"
	"github.com/spiral/roadrunner/v2/plugins/server"
	rrt "github.com/temporalio/roadrunner-temporal"
	"github.com/temporalio/roadrunner-temporal/plugins/temporal"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/worker"
	"sync/atomic"
)

type (
	activityPool interface {
		Start(ctx context.Context, temporal temporal.Temporal) error
		Destroy(ctx context.Context) error
		Workers() []rrWorker.BaseProcess
		ActivityNames() []string
	}

	activityPoolImpl struct {
		dc         converter.DataConverter
		seqID      uint64
		activities []string
		wp         pool.Pool
		tWorkers   []worker.Worker
	}
)

// newActivityPool
func newActivityPool(listener events.Listener, poolConfig poolImpl.Config, server server.Server) (activityPool, error) {
	wp, err := server.NewWorkerPool(
		context.Background(),
		poolConfig,
		map[string]string{"RR_MODE": RRMode},
		listener,
	)

	if err != nil {
		return nil, err
	}

	return &activityPoolImpl{wp: wp}, nil
}

// initWorkers request workers info from underlying PHP and configures temporal workers linked to the pool.
func (pool *activityPoolImpl) Start(ctx context.Context, temporal temporal.Temporal) error {
	pool.dc = temporal.GetDataConverter()

	err := pool.initWorkers(ctx, temporal)
	if err != nil {
		return err
	}

	for i := 0; i < len(pool.tWorkers); i++ {
		err := pool.tWorkers[i].Start()
		if err != nil {
			return err
		}
	}

	return nil
}

// initWorkers request workers info from underlying PHP and configures temporal workers linked to the pool.
func (pool *activityPoolImpl) Destroy(ctx context.Context) error {
	for i := 0; i < len(pool.tWorkers); i++ {
		pool.tWorkers[i].Stop()
	}

	pool.wp.Destroy(ctx)
	return nil
}

func (pool *activityPoolImpl) Workers() []rrWorker.BaseProcess {
	return pool.wp.Workers()
}

func (pool *activityPoolImpl) ActivityNames() []string {
	return pool.activities
}

// initWorkers request workers workflows from underlying PHP and configures temporal workers linked to the pool.
func (pool *activityPoolImpl) initWorkers(ctx context.Context, temporal temporal.Temporal) error {
	workerInfo, err := rrt.GetWorkerInfo(pool.wp, temporal.GetDataConverter())
	if err != nil {
		return err
	}

	pool.activities = make([]string, 0)
	pool.tWorkers = make([]worker.Worker, 0)

	for _, info := range workerInfo {
		w, err := temporal.CreateWorker(info.TaskQueue, info.Options)
		if err != nil {
			return errors.E(errors.Op("createTemporalWorker"), err, pool.Destroy(ctx))
		}

		pool.tWorkers = append(pool.tWorkers, w)
		for _, activityInfo := range info.Activities {
			w.RegisterActivityWithOptions(pool.executeActivity, activity.RegisterOptions{
				Name:                          activityInfo.Name,
				DisableAlreadyRegisteredCheck: false,
			})

			pool.activities = append(pool.activities, activityInfo.Name)
		}
	}

	return nil
}

// executes activity with underlying worker.
func (pool *activityPoolImpl) executeActivity(ctx context.Context, args *common.Payloads) (*common.Payloads, error) {
	var (
		// todo: activity.getHeartBeatDetails
		err  error
		info = activity.GetInfo(ctx)
		msg  = rrt.Message{
			Command: InvokeActivityCommand,
			ID:      atomic.AddUint64(&pool.seqID, 1),
		}
		cmd = InvokeActivity{
			Name: info.ActivityType.Name,
			Info: info,
			Args: args.Payloads,
		}
	)

	// todo: AnyOf in protobuf
	msg.Params, err = jsoniter.Marshal(cmd)
	if err != nil {
		return nil, err
	}

	result, err := rrt.Execute(pool.wp, rrt.Context{TaskQueue: info.TaskQueue}, msg)
	if err != nil {
		return nil, err
	}

	if len(result) != 1 {
		return nil, errors.E(errors.Op("executeActivity"), "invalid activity worker response")
	}

	if result[0].Error != nil {
		if result[0].Error.Message == "doNotCompleteOnReturn" {
			return nil, activity.ErrResultPending
		}

		return nil, errors.E(result[0].Error.Message)
	}

	return &common.Payloads{Payloads: result[0].Result}, nil
}
