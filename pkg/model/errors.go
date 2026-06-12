package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") ||
		strings.Contains(message, "duplicate key value") ||
		strings.Contains(message, "duplicate entry")
}
