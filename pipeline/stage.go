package pipeline

import (
	"context"
	"fmt"
	"sync"
)

type fifo struct {
	proc Processor
}

// FIFO returns a StageRunner that processes incoming payloads in a first-in
// first-out fashion. Each input is passed to the specified processor and its
// output is emitted to the next stage.
func FIFO(proc Processor) StageRunner {
	return fifo{proc: proc}
}

// Run implements StageRunner.
func (r fifo) Run(ctx context.Context, params StageParams) {
	// Block until we are signaled.
	for {
		select {
		// Monitor any cancellation signals, and shut down if present.
		case <-ctx.Done():
			return

		// Otherwise, handle any payloads given to us.
		case payloadIn, ok := <-params.Input():
			// If the channel has been closed, shut down.
			if !ok {
				return
			}

			// Otherwise, begin processing the payload.
			payloadOut, err := r.proc.Process(ctx, payloadIn)
			if err != nil {
				wrappedErr := fmt.Errorf("pipeline stage %d: %w", params.StageIndex(), err)
				maybeEmitError(wrappedErr, params.Error())
				return
			}

			// If the processor did not output a payload for the
			// next stage there is nothing we need to do.
			if payloadOut == nil {
				payloadIn.MarkAsProcessed()
				continue
			}

			// Output processed data
			select {
			case params.Output() <- payloadOut:
			case <-ctx.Done():
				// Asked to cleanly shut down
				return
			}
		}
	}
}

type fixedWorkerPool struct {
	fifos []StageRunner
}

// FixedWorkerPool returns a StageRunner that spins up a pool containing
// numWorkers to process incoming payloads in parallel and emit their outputs
// to the next stage.
func FixedWorkerPool(proc Processor, numWorkers int) StageRunner {
	if numWorkers <= 0 {
		panic("FixedWorkerPool: numWorkers must be > 0")
	}

	fifos := make([]StageRunner, numWorkers)
	for i := 0; i < numWorkers; i++ {
		// All workers are listening on the same input channel and writing to the same output channel.
		fifos[i] = FIFO(proc)
	}

	return &fixedWorkerPool{fifos: fifos}
}

// Run implements StageRunner.
func (p *fixedWorkerPool) Run(ctx context.Context, params StageParams) {
	var wg sync.WaitGroup

	// Spin up each worker in the pool and wait for them to exit
	for i := 0; i < len(p.fifos); i++ {
		wg.Add(1)
		go func(fifoIndex int) {
			// Run will block until a cancellation signal is sent or the input channel is closed.
			p.fifos[fifoIndex].Run(ctx, params)
			wg.Done()
		}(i)
	}

	// Wait for all workers to finish running before returning.
	wg.Wait()
}

type dynamicWorkerPool struct {
	proc      Processor
	tokenPool chan struct{}
}

// DynamicWorkerPool returns a StageRunner that maintains a dynamic worker pool
// that can scale up to maxWorkers for processing incoming inputs in parallel
// and emitting their outputs to the next stage.
func DynamicWorkerPool(proc Processor, maxWorkers int) StageRunner {
	if maxWorkers <= 0 {
		panic("DynamicWorkerPool: maxWorkers must be > 0")
	}

	tokenPool := make(chan struct{}, maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		tokenPool <- struct{}{}
	}

	return &dynamicWorkerPool{proc: proc, tokenPool: tokenPool}
}

// Run implements StageRunner.
func (p *dynamicWorkerPool) Run(ctx context.Context, params StageParams) {
stop:
	for {
		select {
		case <-ctx.Done():
			// Asked to cleanly shut down
			break stop
		case payloadIn, ok := <-params.Input():
			if !ok {
				break stop
			}

			// Wait for a token to become available before processing this payload.
			var token struct{}
			select {
			case token = <-p.tokenPool:
			case <-ctx.Done():
				break stop
			}

			// Once we obtain a token, we can begin processing.
			go func(payloadIn Payload, token struct{}) {
				// Return the token to the pool when we finish.
				defer func() { p.tokenPool <- token }()

				// Normal payload-processing code.
				payloadOut, err := p.proc.Process(ctx, payloadIn)
				if err != nil {
					wrappedErr := fmt.Errorf("pipeline stage %d: %w", params.StageIndex(), err)
					maybeEmitError(wrappedErr, params.Error())
					return
				}

				// If the processor did not output a payload for the
				// next stage there is nothing we need to do.
				if payloadOut == nil {
					payloadIn.MarkAsProcessed()
					return
				}

				// Output processed data
				select {
				case params.Output() <- payloadOut:
				case <-ctx.Done():
				}
			}(payloadIn, token)
		}
	}

	// Wait until all workers turn in their tokens.
	// Alternatively, a sync.WaitGroup could be used.
	for i := 0; i < cap(p.tokenPool); i++ {
		<-p.tokenPool
	}
}

type broadcast struct {
	fifos []StageRunner
}

// Broadcast returns a StageRunner that passes a copy of each incoming payload
// to all specified processors and emits their outputs to the next stage.
// At least one Processor must be given.
func Broadcast(proc Processor, procs ...Processor) StageRunner {
	// This uses a default argument to enforce that at least 1 processor is given.
	// See the original implementation for an alternative solution (checks the length of procs).
	fifos := make([]StageRunner, len(procs)+1)
	fifos[0] = FIFO(proc)
	for i, p := range procs {
		fifos[i+1] = FIFO(p)
	}

	return &broadcast{fifos: fifos}
}

// Run implements StageRunner.
func (b *broadcast) Run(ctx context.Context, params StageParams) {
	var (
		wg   sync.WaitGroup
		inCh = make([]chan Payload, len(b.fifos))
	)

	// Configure a dedicated input channel for each FIFO worker, but keep them wired to the same output and error channels.
	// Start up each worker and add them to the wait group.
	for i := 0; i < len(b.fifos); i++ {
		wg.Add(1)
		inCh[i] = make(chan Payload)
		go func(fifoIndex int) {
			fifoParams := &workerParams{
				stage: params.StageIndex(),
				inCh:  inCh[fifoIndex],
				outCh: params.Output(),
				errCh: params.Error(),
			}
			b.fifos[fifoIndex].Run(ctx, fifoParams)
			wg.Done()
		}(i)
	}

done:
	for {
		// Read incoming payloads and pass them to each FIFO
		select {
		case <-ctx.Done():
			break done
		case payload, ok := <-params.Input():
			if !ok {
				break done
			}

			fifoPayload := payload
			for i := 0; i < len(b.fifos); i++ {
				select {
				case <-ctx.Done():
					break done
				case inCh[i] <- fifoPayload:
					// payload sent to i_th FIFO
				}

				// The first FIFO gets the original payload; everything else gets a copy.
				fifoPayload = payload.Clone()
			}
		}
	}

	// Close input channels and wait for FIFOs to exit
	for _, ch := range inCh {
		close(ch)
	}
	wg.Wait()
}
