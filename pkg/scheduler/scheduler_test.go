package scheduler

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
)

func TestAcquireRequiresEnabledAndDueTask(t *testing.T) {
	previousDB := model.DB
	t.Cleanup(func() {
		model.DB = previousDB
	})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ScheduledTask{}))
	model.DB = db

	now := time.Now()
	future := now.Add(time.Hour)
	disabled := createScheduledTask(t, model.ScheduledTask{
		ClusterName:     "cluster-a",
		Type:            "test",
		Key:             "disabled",
		Enabled:         false,
		ScheduleType:    model.ScheduledTaskScheduleTypeInterval,
		IntervalMinutes: 1,
		NextRunAt:       &now,
	})
	notDue := createScheduledTask(t, model.ScheduledTask{
		ClusterName:     "cluster-a",
		Type:            "test",
		Key:             "not-due",
		Enabled:         true,
		ScheduleType:    model.ScheduledTaskScheduleTypeInterval,
		IntervalMinutes: 1,
		NextRunAt:       &future,
	})
	due := createScheduledTask(t, model.ScheduledTask{
		ClusterName:     "cluster-a",
		Type:            "test",
		Key:             "due",
		Enabled:         true,
		ScheduleType:    model.ScheduledTaskScheduleTypeInterval,
		IntervalMinutes: 1,
		NextRunAt:       &now,
	})

	manager := NewManager()
	require.False(t, manager.acquire(disabled, now))
	require.False(t, manager.acquire(notDue, now))
	require.True(t, manager.acquire(due, now))
}

func createScheduledTask(t *testing.T, task model.ScheduledTask) model.ScheduledTask {
	t.Helper()
	require.NoError(t, model.DB.Create(&task).Error)
	return task
}
