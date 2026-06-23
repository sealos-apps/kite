package ai

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/model"
)

func TestPendingSessionValidateScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("user", model.User{Username: "alice"})
	cs := &cluster.ClientSet{Name: "cluster-a"}

	session := pendingSession{UserKey: "alice", ClusterName: "cluster-a"}
	if err := session.validateScope(c, cs); err != nil {
		t.Fatalf("expected matching scope to pass: %v", err)
	}

	session.UserKey = "bob"
	if err := session.validateScope(c, cs); err == nil || !strings.Contains(err.Error(), "current user or cluster") {
		t.Fatalf("expected user mismatch error, got %v", err)
	}

	session = pendingSession{UserKey: "alice", ClusterName: "cluster-b"}
	if err := session.validateScope(c, cs); err == nil || !strings.Contains(err.Error(), "current user or cluster") {
		t.Fatalf("expected cluster mismatch error, got %v", err)
	}

	session = pendingSession{UserKey: "", ClusterName: "cluster-a"}
	if err := session.validateScope(c, cs); err == nil || !strings.Contains(err.Error(), "missing owner context") {
		t.Fatalf("expected missing owner context error, got %v", err)
	}
}
