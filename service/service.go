package service

import (
	"context"
	"fmt"
	"github.com/hashicorp/go-multierror"
	"sync"
)

// Service describes a service for the Links 'R' Us monolithic application.
type Service interface {
	// Name returns the service name.
	Name() string

	// Run executes the service and blocks until the context gets cancelled
	// or an error occurs.
	Run(context.Context) error
}

// Group is a list of Service instances that can execute in parallel.
type Group []Service

// Run executes all Service instances in the group using the provided context.
// Calls to Run block until all services have completed executing either because
// the context was cancelled or any of the services reported an error.
func (g Group) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancelFn := context.WithCancel(ctx)
	defer cancelFn()

	var wg sync.WaitGroup
	errCh := make(chan error, len(g))
	wg.Add(len(g))
	for _, s := range g {
		go func(s Service) {
			defer wg.Done()
			if err := s.Run(runCtx); err != nil {
				errCh <- fmt.Errorf("%s: %w", s.Name(), err)
				cancelFn()
			}
		}(s)
	}

	// There are only two cases where a service will exit:
	// 	(1) The context is cancelled.
	// 	(2) An error occurs.
	// In both cases, the Done channel will be closed, unblocking this routine.
	<-runCtx.Done()

	// Wait for the cancellation to propagate to all goroutines.
	wg.Wait()

	// Collect and accumulate any reported errors.
	var err error
	close(errCh) // Close the channel so that the loop ends when we read all errors in the buffer.
	for srvErr := range errCh {
		err = multierror.Append(err, srvErr)
	}
	return err
}
