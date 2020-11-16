package main

import (
	"log"

	"github.com/temporalio/roadrunner-temporal/plugins/informer"
	"github.com/temporalio/roadrunner-temporal/plugins/resetter"

	"github.com/spiral/roadrunner/v2/plugins/logger"
	"github.com/spiral/roadrunner/v2/plugins/rpc"
	"github.com/spiral/roadrunner/v2/plugins/server"
	"github.com/temporalio/roadrunner-temporal/plugins/activity"
	"github.com/temporalio/roadrunner-temporal/plugins/temporal"
	"github.com/temporalio/roadrunner-temporal/plugins/workflow"

	"github.com/temporalio/roadrunner-temporal/cmd/cli"
)

func main() {
	err := cli.InitApp(
		// todo: move to root
		&logger.ZapLogger{},

		// Helpers
		&resetter.Plugin{},
		&informer.Plugin{},

		// PHP application init.
		&server.Plugin{},
		&rpc.Plugin{},

		// Temporal extension.
		&temporal.Plugin{},
		&activity.Plugin{},
		&workflow.Plugin{},
	)

	if err != nil {
		log.Fatal(err)
		return
	}

	cli.Execute()
}
