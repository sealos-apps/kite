package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"golang.org/x/net/websocket"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

type PodLogStream struct {
	Pod    corev1.Pod
	Cancel context.CancelFunc
	Done   chan struct{}
}

type BatchLogHandler struct {
	conn      *websocket.Conn
	pods      map[string]*PodLogStream // key: namespace/name
	k8sClient *K8sClient
	opts      *corev1.PodLogOptions
	ctx       context.Context
	cancel    context.CancelFunc
	maxPods   int
	streamSem chan struct{}

	mu             sync.RWMutex
	writeMu        sync.Mutex
	podLimitWarned bool
}

func NewBatchLogHandler(conn *websocket.Conn, client *K8sClient, opts *corev1.PodLogOptions, maxPods, maxConcurrentStreams int) *BatchLogHandler {
	ctx, cancel := context.WithCancel(context.Background())
	if maxPods <= 0 {
		maxPods = 1
	}
	if maxConcurrentStreams <= 0 {
		maxConcurrentStreams = 1
	}
	l := &BatchLogHandler{
		conn:      conn,
		pods:      make(map[string]*PodLogStream),
		k8sClient: client,
		opts:      opts,
		ctx:       ctx,
		cancel:    cancel,
		maxPods:   maxPods,
		streamSem: make(chan struct{}, maxConcurrentStreams),
	}
	return l
}

func (l *BatchLogHandler) StreamLogs(ctx context.Context) {
	// Start heartbeat handler
	go l.heartbeat(ctx)

	// Wait for either external context cancellation or internal cancellation
	select {
	case <-ctx.Done():
		klog.V(1).Info("External context cancelled, stopping BatchLogHandler")
	case <-l.ctx.Done():
		klog.V(1).Info("Internal context cancelled, stopping BatchLogHandler")
	}

	l.Stop()
}

func (l *BatchLogHandler) startPodLogStream(podStream *PodLogStream) {
	pod := podStream.Pod
	select {
	case l.streamSem <- struct{}{}:
	case <-l.ctx.Done():
		return
	}
	defer func() {
		<-l.streamSem
	}()

	podCtx, cancel := context.WithCancel(l.ctx)
	podStream.Cancel = cancel

	defer func() {
		close(podStream.Done)
	}()

	req := l.k8sClient.ClientSet.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, l.opts)
	podLogs, err := req.Stream(podCtx)
	if err != nil {
		_ = l.sendErrorMessage(fmt.Sprintf("Failed to get pod logs for %s: %v", pod.Name, err))
		return
	}
	defer func() {
		_ = podLogs.Close()
	}()

	lw := writerFunc(func(p []byte) (int, error) {
		logString := string(p)
		logLines := strings.SplitSeq(logString, "\n")
		for line := range logLines {
			if line == "" {
				continue
			}
			if l.PodCount() > 1 {
				line = fmt.Sprintf("[%s]: %s", pod.Name, line)
			}
			err := l.sendMessage("log", line)
			if err != nil {
				return 0, err
			}
		}

		return len(p), nil
	})

	_, err = io.Copy(lw, podLogs)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
		_ = l.sendErrorMessage(fmt.Sprintf("Failed to stream pod logs for %s: %v", pod.Name, err))
	}

	_ = l.sendMessage("close", fmt.Sprintf("{\"status\":\"closed\",\"pod\":\"%s\"}", pod.Name))
}

func (l *BatchLogHandler) heartbeat(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			klog.Info("Heartbeat stopping due to context cancellation")
			return
		case <-l.ctx.Done():
			klog.Info("Heartbeat stopping due to internal context cancellation")
			return
		default:
			var temp []byte
			err := websocket.Message.Receive(l.conn, &temp)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					klog.Errorf("WebSocket connection error in heartbeat, cancelling internal context: %v", err)
				}
				l.cancel() // Cancel internal context when connection is lost
				return
			}
			if strings.Contains(string(temp), "ping") {
				err = l.sendMessage("pong", "pong")
				if err != nil {
					klog.Infof("Failed to send pong, cancelling internal context: %v", err)
					l.cancel() // Cancel internal context when send fails
					return
				}
			}
		}
	}
}

// AddPod adds a new pod to the batch log handler and starts streaming its logs
func (l *BatchLogHandler) AddPod(pod corev1.Pod) bool {
	key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)

	l.mu.Lock()
	if _, exists := l.pods[key]; exists {
		l.mu.Unlock()
		return true
	}
	if len(l.pods) >= l.maxPods {
		shouldWarn := !l.podLimitWarned
		l.podLimitWarned = true
		l.mu.Unlock()
		if shouldWarn {
			_ = l.sendMessage("warning", fmt.Sprintf("maximum pod log streams reached: %d", l.maxPods))
		}
		return false
	}

	podStream := &PodLogStream{
		Pod:  pod,
		Done: make(chan struct{}),
	}
	l.pods[key] = podStream
	l.mu.Unlock()

	// Start streaming for this pod
	go l.startPodLogStream(podStream)

	_ = l.sendMessage("pod_added", fmt.Sprintf("{\"pod\":\"%s\",\"namespace\":\"%s\"}",
		pod.Name, pod.Namespace))
	return true
}

// RemovePod removes a pod from the batch log handler and stops streaming its logs
func (l *BatchLogHandler) RemovePod(pod corev1.Pod) {
	key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
	l.mu.Lock()
	podStream, exists := l.pods[key]
	if !exists {
		l.mu.Unlock()
		return
	}
	delete(l.pods, key)
	l.mu.Unlock()

	if podStream.Cancel != nil {
		podStream.Cancel()
	}

	go func() {
		<-podStream.Done
		_ = l.sendMessage("pod_removed", fmt.Sprintf("{\"pod\":\"%s\",\"namespace\":\"%s\"}",
			pod.Name, pod.Namespace))
	}()
}

func (l *BatchLogHandler) Stop() {
	l.mu.Lock()
	podStreams := make([]*PodLogStream, 0, len(l.pods))
	for _, podStream := range l.pods {
		podStreams = append(podStreams, podStream)
	}
	l.pods = make(map[string]*PodLogStream)
	l.mu.Unlock()

	for _, podStream := range podStreams {
		if podStream.Cancel != nil {
			podStream.Cancel()
		}
	}
	l.cancel()
}

func (l *BatchLogHandler) PodCount() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.pods)
}

// writerFunc adapts a function to io.Writer so we can create
// small writers inline inside functions and capture local state.
type writerFunc func([]byte) (int, error)

func (wf writerFunc) Write(p []byte) (int, error) {
	return wf(p)
}

type LogsMessage struct {
	Type string `json:"type"` // "log", "error", "connected", "close"
	Data string `json:"data"`
}

func (l *BatchLogHandler) sendMessage(msgType, data string) error {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()

	msg := LogsMessage{
		Type: msgType,
		Data: data,
	}
	if err := websocket.JSON.Send(l.conn, msg); err != nil {
		return err
	}
	return nil
}

func (l *BatchLogHandler) sendErrorMessage(errMsg string) error {
	return l.sendMessage("error", errMsg)
}
