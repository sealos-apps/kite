package helm

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/helmutil"
)

const uploadMultipartOverheadBytes = 1 << 20

func (h *HelmChartHandler) GetRepositoryUploadConfig(c *gin.Context) {
	config, err := helmutil.LoadRepositoryUploadConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, config)
}

func (h *HelmChartHandler) UploadOCIChart(c *gin.Context) {
	maxBytes, err := helmutil.OCIChartUploadMaxBytes()
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "chart package file is required"})
		return
	}
	defer func() { _ = file.Close() }()

	data, err := readUploadBytes(file, maxBytes)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := helmutil.PushOCIChartArchive(data)
	if err != nil {
		writeUploadError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (h *HelmChartHandler) UploadContainerImage(c *gin.Context) {
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "container image archive file is required"})
		return
	}
	defer func() { _ = file.Close() }()

	archivePath, size, err := writeUploadTempFile(file, maxBytes)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defer func() { _ = os.Remove(archivePath) }()

	result, err := helmutil.PushContainerImageArchive(c.Request.Context(), helmutil.ContainerImageUploadRequest{
		ArchivePath: archivePath,
		Repository:  c.PostForm("repository"),
		Tag:         c.PostForm("tag"),
		Size:        size,
		MaxBytes:    maxBytes,
	})
	if err != nil {
		writeUploadError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func readUploadBytes(file multipart.File, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read uploaded file")
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("uploaded file exceeds %s", formatBytes(maxBytes))
	}
	return data, nil
}

func writeUploadTempFile(file multipart.File, maxBytes int64) (string, int64, error) {
	tempFile, err := os.CreateTemp("", "kite-image-upload-*")
	if err != nil {
		return "", 0, fmt.Errorf("failed to create temporary upload file")
	}
	tempPath := tempFile.Name()
	defer func() { _ = tempFile.Close() }()

	written, err := io.Copy(tempFile, io.LimitReader(file, maxBytes+1))
	if err != nil {
		_ = os.Remove(tempPath)
		return "", 0, fmt.Errorf("failed to save uploaded file")
	}
	if written > maxBytes {
		_ = os.Remove(tempPath)
		return "", 0, fmt.Errorf("uploaded file exceeds %s", formatBytes(maxBytes))
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return "", 0, fmt.Errorf("failed to save uploaded file")
	}
	return filepath.Clean(tempPath), written, nil
}

func writeUploadError(c *gin.Context, err error) {
	if helmutil.IsUploadClientError(err) || errors.Is(err, http.ErrBodyReadAfterClose) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func writeMaxBytesError(c *gin.Context, err error, maxBytes int64) bool {
	var maxBytesError *http.MaxBytesError
	if !errors.As(err, &maxBytesError) {
		return false
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("uploaded file exceeds %s", formatBytes(maxBytes))})
	return true
}

func cleanupMultipartForm(req *http.Request) {
	if req.MultipartForm != nil {
		_ = req.MultipartForm.RemoveAll()
	}
}

func formatBytes(bytes int64) string {
	const mib = 1024 * 1024
	if bytes%mib == 0 {
		return fmt.Sprintf("%dMiB", bytes/mib)
	}
	return fmt.Sprintf("%d bytes", bytes)
}
