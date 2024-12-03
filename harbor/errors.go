package harbor

import (
	"errors"
	"fmt"
)

type HarborErrors struct {
	ErrorList []HarborError `json:"errors"`
}

func (he HarborErrors) Error() string {
	for _, e := range he.ErrorList {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return "unknown error"
}

type HarborError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

const (
	ErrorCodeNotFound = "NOT_FOUND"
)

func IsErrorCode(err error, code string) bool {
	if err == nil {
		return false
	}
	harbore := HarborErrors{}
	if errors.As(err, &harbore) {
		for _, e := range harbore.ErrorList {
			if e.Code == code {
				return true
			}
		}
	}
	return false
}
