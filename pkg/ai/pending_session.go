package ai

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/model"
	"k8s.io/klog/v2"
)

const pendingSessionTTL = 15 * time.Minute

type pendingToolCall struct {
	ID   string
	Name string
	Args map[string]interface{}
}

type pendingSession struct {
	UserKey           string
	ClusterName       string
	Provider          string
	SystemPrompt      string
	OpenAIMessages    []openai.ChatCompletionMessageParamUnion
	AnthropicMessages []anthropic.MessageParam
	ToolCall          pendingToolCall
	ExpiresAt         time.Time
}

type pendingSessionStore struct{}

var agentPendingSessions = &pendingSessionStore{}

func (s *pendingSessionStore) save(session pendingSession) string {
	sessionID := newPendingSessionID()
	session.ExpiresAt = time.Now().Add(pendingSessionTTL)

	dbSession := &model.PendingSession{
		SessionID:    sessionID,
		UserKey:      session.UserKey,
		ClusterName:  session.ClusterName,
		Provider:     session.Provider,
		SystemPrompt: session.SystemPrompt,
		ToolCallID:   session.ToolCall.ID,
		ToolCallName: session.ToolCall.Name,
		ExpiresAt:    session.ExpiresAt,
	}

	// Marshal messages and args
	if err := dbSession.OpenAIMessages.Marshal(session.OpenAIMessages); err != nil {
		klog.Errorf("Failed to marshal OpenAI messages: %v", err)
		return ""
	}
	if err := dbSession.AnthropicMessages.Marshal(session.AnthropicMessages); err != nil {
		klog.Errorf("Failed to marshal Anthropic messages: %v", err)
		return ""
	}
	if err := dbSession.ToolCallArgs.Marshal(session.ToolCall.Args); err != nil {
		klog.Errorf("Failed to marshal tool call args: %v", err)
		return ""
	}

	if err := model.SavePendingSession(dbSession); err != nil {
		klog.Errorf("Failed to save pending session: %v", err)
		return ""
	}

	// Cleanup expired sessions asynchronously
	go func() {
		if err := model.CleanupExpiredPendingSessions(); err != nil {
			klog.V(4).Infof("Failed to cleanup expired pending sessions: %v", err)
		}
	}()

	return sessionID
}

func (s *pendingSessionStore) load(sessionID string) (pendingSession, error) {
	dbSession, err := model.GetPendingSession(sessionID)
	if err != nil {
		return pendingSession{}, fmt.Errorf("pending action not found or expired")
	}

	return pendingSessionFromModel(dbSession)
}

func (s *pendingSessionStore) delete(sessionID string) {
	if err := model.DeletePendingSession(sessionID); err != nil {
		klog.Warningf("Failed to delete pending session %s: %v", sessionID, err)
	}
}

func (s *pendingSessionStore) take(sessionID string) (pendingSession, error) {
	session, err := s.load(sessionID)
	if err != nil {
		return pendingSession{}, err
	}
	s.delete(sessionID)
	return session, nil
}

func pendingSessionFromModel(dbSession *model.PendingSession) (pendingSession, error) {
	session := pendingSession{
		UserKey:      dbSession.UserKey,
		ClusterName:  dbSession.ClusterName,
		Provider:     dbSession.Provider,
		SystemPrompt: dbSession.SystemPrompt,
		ExpiresAt:    dbSession.ExpiresAt,
		ToolCall: pendingToolCall{
			ID:   dbSession.ToolCallID,
			Name: dbSession.ToolCallName,
		},
	}

	var err error

	// Unmarshal messages and args directly in the AI package
	if err = dbSession.OpenAIMessages.Unmarshal(&session.OpenAIMessages); err != nil {
		return pendingSession{}, fmt.Errorf("failed to unmarshal OpenAI messages: %w", err)
	}
	if err = dbSession.AnthropicMessages.Unmarshal(&session.AnthropicMessages); err != nil {
		return pendingSession{}, fmt.Errorf("failed to unmarshal Anthropic messages: %w", err)
	}
	if err = dbSession.ToolCallArgs.Unmarshal(&session.ToolCall.Args); err != nil {
		return pendingSession{}, fmt.Errorf("failed to unmarshal tool call args: %w", err)
	}

	return session, nil
}

func buildPendingSessionScope(c *gin.Context, cs *cluster.ClientSet) (string, string) {
	userKey := ""
	if user, ok := currentUserFromGin(c); ok {
		userKey = user.Key()
	}
	clusterName := ""
	if cs != nil {
		clusterName = cs.Name
	}
	return userKey, clusterName
}

func (s pendingSession) validateScope(c *gin.Context, cs *cluster.ClientSet) error {
	userKey, clusterName := buildPendingSessionScope(c, cs)
	if s.UserKey == "" || s.ClusterName == "" {
		return fmt.Errorf("pending session is missing owner context")
	}
	if s.UserKey != userKey || s.ClusterName != clusterName {
		return fmt.Errorf("pending session does not belong to the current user or cluster")
	}
	return nil
}

func newPendingSessionID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return fmt.Sprintf("pending-%d", time.Now().UnixNano())
}
