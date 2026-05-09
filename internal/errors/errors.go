package errors

import (
	"fmt"
)

type ErrorCode string

const (
	CodeQuotaExceeded       ErrorCode = "QUOTA_EXCEEDED"
	CodeProviderUnavailable ErrorCode = "PROVIDER_UNAVAILABLE"
	CodeInternalFailure     ErrorCode = "INTERNAL_FAILURE"
	CodeQueryInvalid        ErrorCode = "QUERY_INVALID"
	CodePolicyViolation     ErrorCode = "POLICY_VIOLATION"
)

type RecoveryType string

const (
	RecoveryWait     RecoveryType = "wait"
	RecoveryRephrase RecoveryType = "rephrase"
)

type RecoveryAction struct {
	Type        RecoveryType `json:"type"`
	WaitSeconds int          `json:"wait_after,omitempty"`
	Suggestion  string       `json:"suggestion,omitempty"`
}

type AppError struct {
	Code        ErrorCode       `json:"code"`
	Source      string          `json:"source"` // "llm", "search", "storage", etc.
	UserMessage string          `json:"user_message"`
	Recovery    *RecoveryAction `json:"recovery,omitempty"`
	Telemetry   map[string]any  `json:"telemetry,omitempty"`
	Err         error           `json:"-"` // The original error for logging
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.UserMessage, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.UserMessage)
}

func New(code ErrorCode, source, msg string, err error) *AppError {
	return &AppError{
		Code:        code,
		Source:      source,
		UserMessage: msg,
		Err:         err,
	}
}
