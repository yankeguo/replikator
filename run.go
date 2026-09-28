package replikator

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

// RunOptions controls the configuration supervisor.
type RunOptions struct {
	TaskOptions
	// PollInterval is how often the configuration directory is checked.
	// Zero uses the default of 10 seconds. Polling is used because mounted
	// ConfigMaps do not reliably deliver filesystem notifications.
	PollInterval time.Duration
}

func (o RunOptions) pollInterval() time.Duration {
	if o.PollInterval > 0 {
		return o.PollInterval
	}
	return defaultPollInterval
}

// Run loads tasks from dir and replicates until ctx is cancelled.
// When the directory changes, tasks are replaced. A reload that fails to parse
// leaves the previous tasks running.
func Run(ctx context.Context, dir string, opts RunOptions) error {
	digest, err := DigestTaskDefinitionsFromDir(dir)
	if err != nil {
		return err
	}
	tasks, err := LoadTasks(dir)
	if err != nil {
		return err
	}
	logrus.WithField("count", len(tasks)).WithField("digest", digest).Info("tasks loaded")

	// The deferred shutdown reads these variables after they are reassigned on reload.
	cancel, done := startTasks(ctx, tasks, opts.TaskOptions)
	defer func() {
		cancel()
		<-done
	}()

	ticker := time.NewTicker(opts.pollInterval())
	defer ticker.Stop()

	var lastDigestErr, lastReloadErr string
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		next, err := DigestTaskDefinitionsFromDir(dir)
		if err != nil {
			if msg := err.Error(); msg != lastDigestErr {
				logrus.WithError(err).Error("failed to digest task definitions")
				lastDigestErr = msg
			}
			continue
		}
		lastDigestErr = ""
		if next == digest {
			continue
		}

		tasks, err = LoadTasks(dir)
		if err != nil {
			if msg := err.Error(); msg != lastReloadErr {
				logrus.WithError(err).Error("failed to reload task definitions; keeping previous tasks")
				lastReloadErr = msg
			}
			continue
		}
		lastReloadErr = ""
		digest = next
		logrus.WithField("count", len(tasks)).WithField("digest", digest).Info("tasks reloaded")

		cancel()
		<-done
		if ctx.Err() != nil {
			return nil
		}
		cancel, done = startTasks(ctx, tasks, opts.TaskOptions)
	}
}

func startTasks(ctx context.Context, tasks TaskList, opts TaskOptions) (context.CancelFunc, <-chan struct{}) {
	taskCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		tasks.NewSessions(opts).Run(taskCtx)
	}()
	return cancel, done
}
