package helm

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

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

func (h *HelmChartHandler) GetOfflineBundleConfig(c *gin.Context) {
	config, err := helmutil.LoadOfflineBundleTransferConfig()
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

func (h *HelmChartHandler) ImportOfflineBundle(c *gin.Context) {
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
	defer func() { _ = os.Remove(bundlePath) }()

	result, err := helmutil.ImportOfflineBundle(c.Request.Context(), bundlePath, helmutil.BundleImportOptions{
		MaxBytes: maxBytes,
	})
	if err != nil {
		writeUploadError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (h *HelmChartHandler) ExportOfflineBundle(c *gin.Context) {
	var req struct {
		Apps []helmutil.BundleExportApplication `json:"apps"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid offline application bundle export request"})
		return
	}
	outputFile, err := os.CreateTemp("", "kite-offline-bundle-export-*.kiteapp.tar.gz")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create temporary export file"})
		return
	}
	outputPath := outputFile.Name()
	if err := outputFile.Close(); err != nil {
		_ = os.Remove(outputPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create temporary export file"})
		return
	}
	defer func() { _ = os.Remove(outputPath) }()

	manifest, err := helmutil.ExportOfflineBundle(c.Request.Context(), outputPath, helmutil.BundleExportOptions{
		Apps: req.Apps,
	})
	if err != nil {
		writeUploadError(c, err)
		return
	}
	filename := offlineBundleDownloadName(manifest)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Content-Type", "application/gzip")
	c.File(outputPath)
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

func offlineBundleDownloadName(manifest helmutil.OfflineBundleManifest) string {
	name := "kite-offline-apps"
	if len(manifest.Apps) == 1 {
		name = fmt.Sprintf("%s-%s", manifest.Apps[0].Name, manifest.Apps[0].Version)
	}
	name = sanitizeDownloadName(name)
	if name == "" {
		name = "kite-offline-apps"
	}
	return fmt.Sprintf("%s-%s.kiteapp.tar.gz", name, time.Now().UTC().Format("20060102T150405Z"))
}

func sanitizeDownloadName(value string) string {
	out := make([]rune, 0, len(value))
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			out = append(out, r)
		case r >= 'A' && r <= 'Z':
			out = append(out, r)
		case r >= '0' && r <= '9':
			out = append(out, r)
		case r == '-', r == '_', r == '.':
			out = append(out, r)
		default:
			out = append(out, '-')
		}
	}
	return string(out)
}
