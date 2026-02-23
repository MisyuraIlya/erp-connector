package hasavshevet

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"

	"erp-connector/internal/logger"
)

const defaultQueueSize = 64

// JobStatus represents the lifecycle state of an enqueued order job.
type JobStatus string

const (
	JobStatusQueued  JobStatus = "queued"
	JobStatusRunning JobStatus = "running"
	JobStatusDone    JobStatus = "done"
	JobStatusFailed  JobStatus = "failed"
)

// JobResult holds the outcome of a processed order job.
type JobResult struct {
	ID           string
	Status       JobStatus
	OrderNumber  int64
	WrittenFiles []string
	Err          error
}

type orderJob struct {
	id  string
	req OrderRequest
}

// OrderQueue is a single-worker async queue for Hasavshevet send-order jobs.
//
// Using a single worker guarantees that only one goroutine writes to
// IMOVEIN.doc/.prm and executes has.exe at a time, preventing file collisions.
// Jobs are processed in FIFO order.
type OrderQueue struct {
	ch     chan orderJob
	sender *Sender
	log    logger.LoggerService

	mu   sync.RWMutex
	jobs map[string]*JobResult
}

// NewOrderQueue creates a new queue. Call Start to begin processing.
func NewOrderQueue(sender *Sender, log logger.LoggerService) *OrderQueue {
	return &OrderQueue{
		ch:     make(chan orderJob, defaultQueueSize),
		sender: sender,
		log:    log,
		jobs:   make(map[string]*JobResult),
	}
}

// Start launches the single background worker goroutine.
// The goroutine exits when ctx is cancelled or Stop is called.
func (q *OrderQueue) Start(ctx context.Context) {
	go q.run(ctx)
}

func (q *OrderQueue) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-q.ch:
			if !ok {
				return
			}
			q.setStatus(job.id, JobStatusRunning, 0, nil, nil)
			result, err := q.sender.ProcessOrder(ctx, job.req)
			if err != nil {
				q.log.Error(fmt.Sprintf("order job %s failed", job.id), err)
				q.setStatus(job.id, JobStatusFailed, 0, nil, err)
			} else {
				q.log.Success(fmt.Sprintf("order job %s done orderNumber=%d files=%v", job.id, result.OrderNumber, result.WrittenFiles))
				q.setStatus(job.id, JobStatusDone, result.OrderNumber, result.WrittenFiles, nil)
			}
		}
	}
}

// Submit enqueues an order request and returns a job ID.
// Returns an error if the queue is full.
func (q *OrderQueue) Submit(req OrderRequest) (string, error) {
	jobID := newJobID()
	job := orderJob{id: jobID, req: req}

	q.mu.Lock()
	q.jobs[jobID] = &JobResult{ID: jobID, Status: JobStatusQueued}
	q.mu.Unlock()

	select {
	case q.ch <- job:
		return jobID, nil
	default:
		q.mu.Lock()
		delete(q.jobs, jobID)
		q.mu.Unlock()
		return "", fmt.Errorf("order queue full (capacity %d)", defaultQueueSize)
	}
}

// Status returns the current result for a job ID, or false if not found.
func (q *OrderQueue) Status(jobID string) (*JobResult, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	r, ok := q.jobs[jobID]
	return r, ok
}

// Stop closes the job channel, causing the worker to exit after the current job.
func (q *OrderQueue) Stop() {
	close(q.ch)
}

func (q *OrderQueue) setStatus(id string, status JobStatus, orderNumber int64, files []string, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs[id] = &JobResult{
		ID:           id,
		Status:       status,
		OrderNumber:  orderNumber,
		WrittenFiles: files,
		Err:          err,
	}
}

func newJobID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
