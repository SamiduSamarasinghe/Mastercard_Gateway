package handlers

import (
	"net/http"
	"time"

	"pg-backend/internal/worker"

	"github.com/gin-gonic/gin"
)

type WorkerHandler struct {
	workerManager *worker.WorkerManager
}

func NewWorkerHandler(workerManager *worker.WorkerManager) *WorkerHandler {
	return &WorkerHandler{
		workerManager: workerManager,
	}
}

// GetWorkerStatus returns the status of all workers
func (h *WorkerHandler) GetWorkerStatus(c *gin.Context) {
	status := h.workerManager.GetWorkerStatus()
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"workers":   status,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// RestartWorkers restarts all workers (admin only)
func (h *WorkerHandler) RestartWorkers(c *gin.Context) {
	// In production, add authentication here
	h.workerManager.StopAll()

	// Wait a moment
	time.Sleep(1 * time.Second)

	// Restart
	if err := h.workerManager.StartAll(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to restart workers",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Workers restarted successfully",
	})
}
