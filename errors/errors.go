package errors

import (
	"errors"
	"fmt"
	"net/http"
)

const (
	StatusSuccess = "Success"
	StatusFailure = "Failure"
)

const (
	StatusReasonUnknown               StatusReason = ""
	StatusReasonNotFound              StatusReason = "NotFound"
	StatusReasonAlreadyExists         StatusReason = "AlreadyExists"
	StatusReasonInvalid               StatusReason = "Invalid"
	StatusReasonUnauthorized          StatusReason = "Unauthorized"
	StatusReasonForbidden             StatusReason = "Forbidden"
	StatusReasonConflict              StatusReason = "Conflict"
	StatusReasonBadRequest            StatusReason = "BadRequest"
	StatusReasonInternalError         StatusReason = "InternalError"
	StatusReasonNotImplemented        StatusReason = "NotImplemented"
	StatusReasonUnsupported           StatusReason = "Unsupported"
	StatusReasonTooManyRequests       StatusReason = "TooManyRequests"
	StatusReasonRequestEntityTooLarge StatusReason = "RequestEntityTooLarge"
	StatusReasonResourceExpired       StatusReason = "ResourceExpired"
	StatusReasonServiceUnavailable    StatusReason = "ServiceUnavailable"
)

type StatusReason string

type Status struct {
	// Status of the operation.
	// One of: "Success" or "Failure".
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Status string `json:"status,omitempty"`
	// Suggested HTTP return code for this status, 0 if not set.
	// +optional
	Code int32 `json:"code,omitempty"`
	// A human-readable description of the status of this operation.
	// +optional
	Message string `json:"message,omitempty"`
	// A machine-readable description of why this operation is in the
	// "Failure" status. If this value is empty there
	// is no information available. A Reason clarifies an HTTP status
	// code but does not override it.
	// +optional
	Reason StatusReason `json:"reason,omitempty"`
}

func (s *Status) Error() string {
	return s.Message
}

func NewOK() *Status {
	return &Status{Status: StatusSuccess, Code: http.StatusOK, Reason: StatusReasonUnknown, Message: "OK"}
}

func NewAlreadyExists(resource, name string) *Status {
	message := fmt.Sprintf("%s %q already exists", resource, name)
	return &Status{Status: StatusFailure, Code: http.StatusConflict, Reason: StatusReasonAlreadyExists, Message: message}
}

func NewNotFound(resource, name string) *Status {
	message := fmt.Sprintf("%s %q not found", resource, name)
	return &Status{Status: StatusFailure, Code: http.StatusNotFound, Reason: StatusReasonNotFound, Message: message}
}

func NewUnauthorized(reason string) *Status {
	message := reason
	if len(message) == 0 {
		message = "not authorized"
	}
	return &Status{Status: StatusFailure, Code: http.StatusUnauthorized, Reason: StatusReasonUnauthorized, Message: message}
}

func NewForbidden(resource, name string, err error) *Status {
	var message string
	if resource == "" && name == "" {
		message = fmt.Sprintf("forbidden: %v", err)
	} else if name == "" {
		message = fmt.Sprintf("%s is forbidden: %v", resource, err)
	} else {
		message = fmt.Sprintf("%s %q is forbidden: %v", resource, name, err)
	}
	return &Status{Status: StatusFailure, Code: http.StatusForbidden, Reason: StatusReasonForbidden, Message: message}
}

func NewBadRequest(reason string) *Status {
	return &Status{Status: StatusFailure, Code: http.StatusBadRequest, Reason: StatusReasonBadRequest, Message: reason}
}

func NewInvalid(resource, name string, err error) *Status {
	message := fmt.Sprintf("invalid %s %q: %v", resource, name, err)
	return &Status{Status: StatusFailure, Code: http.StatusBadRequest, Reason: StatusReasonInvalid, Message: message}
}

func NewConflict(resource, name string, err error) *Status {
	message := fmt.Sprintf("Operation cannot be fulfilled on %s %q: %v", resource, name, err)
	return &Status{Status: StatusFailure, Code: http.StatusConflict, Reason: StatusReasonConflict, Message: message}
}

func NewTooManyRequests(message string, retryAfterSeconds int) *Status {
	return &Status{Status: StatusFailure, Code: http.StatusTooManyRequests, Reason: StatusReasonTooManyRequests, Message: message}
}

func NewRequestEntityTooLarge(reason string) *Status {
	message := fmt.Sprintf("Request entity too large: %s", reason)
	return &Status{Status: StatusFailure, Code: http.StatusRequestEntityTooLarge, Reason: StatusReasonRequestEntityTooLarge, Message: message}
}

func NewNotImplemented(reason string) *Status {
	return &Status{Status: StatusFailure, Code: http.StatusNotImplemented, Reason: StatusReasonNotImplemented, Message: reason}
}

func NewUnsupported(reason string) *Status {
	return &Status{Status: StatusFailure, Code: http.StatusNotImplemented, Reason: StatusReasonNotImplemented, Message: reason}
}

func NewInternalError(err error) *Status {
	message := fmt.Sprintf("Internal error occurred: %v", err)
	return &Status{Status: StatusFailure, Code: http.StatusInternalServerError, Reason: StatusReasonInternalError, Message: message}
}

func NewRequestEntityTooLargeError(reason string) *Status {
	message := fmt.Sprintf("Request entity too large: %s", reason)
	return &Status{Status: StatusFailure, Code: http.StatusRequestEntityTooLarge, Reason: StatusReasonRequestEntityTooLarge, Message: message}
}

func NewResourceExpired(resource, msaage string) *Status {
	if resource == "" {
		return &Status{Status: StatusFailure, Code: http.StatusGone, Reason: StatusReasonResourceExpired, Message: msaage}
	}
	message := fmt.Sprintf("%s is expired: %v", resource, msaage)
	return &Status{Status: StatusFailure, Code: http.StatusGone, Reason: StatusReasonResourceExpired, Message: message}
}

func NewServiceUnavailable(reason string) *Status {
	return &Status{Status: StatusFailure, Code: http.StatusServiceUnavailable, Reason: StatusReasonServiceUnavailable, Message: reason}
}

func NewCustomError(code int, reason StatusReason, message string) *Status {
	return &Status{Status: StatusFailure, Code: int32(code), Reason: reason, Message: message}
}

func IsNotFound(err error) bool {
	return ReasonForError(err) == StatusReasonNotFound
}

func IsAlreadyExists(err error) bool {
	return ReasonForError(err) == StatusReasonAlreadyExists
}

func IsConflict(err error) bool {
	return ReasonForError(err) == StatusReasonConflict
}

func ReasonForError(err error) StatusReason {
	if status, ok := err.(*Status); ok || errors.As(err, &status) {
		return status.Reason
	}
	return StatusReasonUnknown
}

func IgnoreNotFound(err error) error {
	if IsNotFound(err) {
		return nil
	}
	return err
}

func IgnoreAlreadyExists(err error) error {
	if IsAlreadyExists(err) {
		return nil
	}
	return err
}

func IsUnauthorized(err error) bool {
	return ReasonForError(err) == StatusReasonUnauthorized
}
