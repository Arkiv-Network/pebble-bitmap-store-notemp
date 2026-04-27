package main

import (
	"context"
	"errors"

	arkivevents "github.com/Arkiv-Network/arkiv-events"
	"github.com/Arkiv-Network/arkiv-events/events"
)

type commitRequest struct {
	batch events.BlockBatch
	done  chan error
}

// CommitQueue bridges per-request RPC calls into the long-running FollowEvents iterator.
// Push blocks until FollowEvents atomically commits the batch to PebbleDB.
type CommitQueue struct {
	ch     chan commitRequest
	closed chan struct{}
}

func NewCommitQueue() *CommitQueue {
	return &CommitQueue{
		ch:     make(chan commitRequest),
		closed: make(chan struct{}),
	}
}

// Push sends a batch and blocks until FollowEvents has committed it.
func (q *CommitQueue) Push(ctx context.Context, batch events.BlockBatch) error {
	req := commitRequest{batch: batch, done: make(chan error, 1)}
	select {
	case q.ch <- req:
	case <-ctx.Done():
		return ctx.Err()
	case <-q.closed:
		return errors.New("commit queue closed")
	}
	select {
	case err := <-req.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close signals the iterator to stop, unblocking any pending Push calls.
func (q *CommitQueue) Close() {
	close(q.closed)
}

// Iterator returns a BatchIterator to be passed to FollowEvents.
// The yield call in the returned iterator suspends until FollowEvents completes
// one full iteration (including pBatch.Commit), so Push returns only after the
// commit is durable. This relies on Go 1.22+ range-over-func semantics.
func (q *CommitQueue) Iterator() arkivevents.BatchIterator {
	return func(yield func(arkivevents.BatchOrError) bool) {
		for {
			var req commitRequest
			select {
			case req = <-q.ch:
			case <-q.closed:
				return
			}
			// yield blocks until FollowEvents finishes the loop body (commit included).
			cont := yield(arkivevents.BatchOrError{Batch: req.batch})
			if cont {
				req.done <- nil
			} else {
				// FollowEvents returned an error — signal the waiting RPC caller.
				req.done <- errors.New("batch processing failed")
				return
			}
		}
	}
}
