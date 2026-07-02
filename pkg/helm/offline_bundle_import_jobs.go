package helm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/helmutil"
)

const maxOfflineBundleImportJobs = 100

type offlineBundleImportJobStatus string

const (
	offlineBundleImportQueued    offlineBundleImportJobStatus = "queued"
	offlineBundleImportRunning   offlineBundleImportJobStatus = "running"
	offlineBundleImportSucceeded offlineBundleImportJobStatus = "succeeded"
	offlineBundleImportFailed    offlineBundleImportJobStatus = "failed"
)

type offlineBundleImportFunc func(context.Context, string, helmutil.BundleImportOptions) (helmutil.BundleImportResult, error)

var helmutilImportOfflineBundle offlineBundleImportFunc = helmutil.ImportOfflineBundle

type offlineBundleImportJob struct {
	ID          string                       `json:"id"`
	Status      offlineBundleImportJobStatus `json:"status"`
	CreatedAt   time.Time                    `json:"createdAt"`
	UpdatedAt   time.Time                    `json:"updatedAt"`
	CompletedAt *time.Time                   `json:"completedAt,omitempty"`
	Result      *helmutil.BundleImportResult `json:"result,omitempty"`
	Error       string                       `json:"error,omitempty"`
}

type offlineBundleImportJobStore struct {
	mu         sync.RWMutex
	jobs       map[string]*offlineBundleImportJob
	order      []string
	importFunc offlineBundleImportFunc
}

func newOfflineBundleImportJobStore(importFunc offlineBundleImportFunc) *offlineBundleImportJobStore {
	if importFunc == nil {
		importFunc = helmutil.ImportOfflineBundle
	}
	return &offlineBundleImportJobStore{
		jobs:       map[string]*offlineBundleImportJob{},
		importFunc: importFunc,
	}
}

func (s *offlineBundleImportJobStore) start(bundlePath string, maxBytes int64) (offlineBundleImportJob, error) {
	id, err := newOfflineBundleImportJobID()
	if err != nil {
		return offlineBundleImportJob{}, err
	}
	now := time.Now().UTC()
	job := &offlineBundleImportJob{
		ID:        id,
		Status:    offlineBundleImportQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.mu.Lock()
	s.jobs[id] = job
	s.order = append(s.order, id)
	s.trimLocked()
	s.mu.Unlock()

	go s.run(id, bundlePath, maxBytes)

	return cloneOfflineBundleImportJob(job), nil
}

func (s *offlineBundleImportJobStore) run(id, bundlePath string, maxBytes int64) {
	defer func() { _ = os.Remove(bundlePath) }()
	s.update(id, func(job *offlineBundleImportJob, now time.Time) {
		job.Status = offlineBundleImportRunning
		job.UpdatedAt = now
	})

	result, err := s.importFunc(context.Background(), bundlePath, helmutil.BundleImportOptions{
		MaxBytes: maxBytes,
	})
	completedAt := time.Now().UTC()
	s.update(id, func(job *offlineBundleImportJob, _ time.Time) {
		job.UpdatedAt = completedAt
		job.CompletedAt = &completedAt
		if err != nil {
			job.Status = offlineBundleImportFailed
			job.Error = err.Error()
			return
		}
		job.Status = offlineBundleImportSucceeded
		job.Result = &result
	})
}

func (s *offlineBundleImportJobStore) get(id string) (offlineBundleImportJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		return offlineBundleImportJob{}, false
	}
	return cloneOfflineBundleImportJob(job), true
}

func (s *offlineBundleImportJobStore) update(id string, update func(*offlineBundleImportJob, time.Time)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return
	}
	update(job, time.Now().UTC())
	s.trimLocked()
}

func (s *offlineBundleImportJobStore) trimLocked() {
	for len(s.order) > maxOfflineBundleImportJobs {
		removed := false
		for index, id := range s.order {
			job := s.jobs[id]
			if job == nil || isOfflineBundleImportJobTerminal(job.Status) {
				delete(s.jobs, id)
				s.order = append(s.order[:index], s.order[index+1:]...)
				removed = true
				break
			}
		}
		if !removed {
			return
		}
	}
}

func isOfflineBundleImportJobTerminal(status offlineBundleImportJobStatus) bool {
	return status == offlineBundleImportSucceeded || status == offlineBundleImportFailed
}

func cloneOfflineBundleImportJob(job *offlineBundleImportJob) offlineBundleImportJob {
	out := *job
	if job.CompletedAt != nil {
		completedAt := *job.CompletedAt
		out.CompletedAt = &completedAt
	}
	if job.Result != nil {
		result := *job.Result
		result.Apps = append([]helmutil.BundleImportAppResult(nil), job.Result.Apps...)
		out.Result = &result
	}
	return out
}

func newOfflineBundleImportJobID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("failed to create import job id")
	}
	return hex.EncodeToString(data[:]), nil
}

func (h *HelmChartHandler) StartOfflineBundleImportJob(c *gin.Context) {
	maxBytes, err := helmutil.ContainerImageUploadMaxBytes()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes+uploadMultipartOverheadBytes)
	defer cleanupMultipartForm(c.Request)
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		if writeMaxBytesError(c, err, maxBytes) {
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "offline application bundle file is required"})
		return
	}
	defer func() { _ = file.Close() }()

	bundlePath, _, err := writeUploadTempFile(file, maxBytes)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	job, err := h.offlineBundleJobs.start(bundlePath, maxBytes)
	if err != nil {
		_ = os.Remove(bundlePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, job)
}

func (h *HelmChartHandler) GetOfflineBundleImportJob(c *gin.Context) {
	job, ok := h.offlineBundleJobs.get(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "offline application bundle import job not found"})
		return
	}
	c.JSON(http.StatusOK, job)
}
