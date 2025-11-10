package validation

import (
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/util/validation"
	"xiaoshiai.cn/common/errors"
)

var (
	// RestricNameRegexp is the most limited name regexp
	// it is used for tenant name.
	// 1-25 length, start with a-z, only a-z0-9
	RestrictNameRegexp = regexp.MustCompile(`^[a-z][a-z0-9]{0,24}$`)

	// NameRegexp is the most common name regexp
	// 1-64 length, start and end with a-z0-9, a-z0-9.-_ in the middle
	NameRegexp = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9_.-]{0,62}[a-zA-Z0-9])?$`)
)

func ValidateDNS1123Name(name string) error {
	if errmsgs := validation.IsDNS1123Label(name); len(errmsgs) > 0 {
		return errors.NewBadRequest(fmt.Sprintf("invalid DNS1123 name %s: %v", name, errmsgs))
	}
	return nil
}

func ValidateName(name string) error {
	return ValidateMatchRegexp(name, NameRegexp)
}

func ValidateRestrictName(name string) error {
	return ValidateMatchRegexp(name, RestrictNameRegexp)
}

func ValidateMatchRegexp(name string, regxp *regexp.Regexp) error {
	if !regxp.MatchString(name) {
		return errors.NewBadRequest(fmt.Sprintf("invalid name %s, must match %s", name, regxp.String()))
	}
	return nil
}
