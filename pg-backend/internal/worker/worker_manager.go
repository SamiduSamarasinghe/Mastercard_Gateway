package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

type WorkerManager struct {
	workers []*BillingWorker
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewWorkerManager() *WorkerManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerManager{
		workers: make([]*BillingWorker, 0),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// RegisterWorker adds a worker to the manager
func (m *WorkerManager) RegisterWorker(worker *BillingWorker) {
	m.workers = append(m.workers, worker)
}

// StartAll starts all registered workers
func (m *WorkerManager) StartAll() error {
	for _, worker := range m.workers {
		m.wg.Add(1)
		go func(w *BillingWorker) {
			defer m.wg.Done()
			if err := w.Start(m.ctx); err != nil {
				// Log error but continue with other workers
				log.Printf("Worker error: %v", err)
			}
		}(worker)
	}

	return nil
}

// StopAll gracefully stops all workers
func (m *WorkerManager) StopAll() {
	m.cancel()
	m.wg.Wait()

	// Give workers time to clean up
	time.Sleep(2 * time.Second)

	for _, worker := range m.workers {
		worker.Stop()
	}
}

// GetWorkerStatus returns status of all workers
func (m *WorkerManager) GetWorkerStatus() map[string]interface{} {
	status := make(map[string]interface{})
	for i, worker := range m.workers {
		status[fmt.Sprintf("worker_%d", i)] = worker.HealthCheck()
	}
	return status
}
