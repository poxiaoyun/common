package i18n

import "fmt"

type Localizer interface {
	// T translate the message to the target language.
	// it works like fmt.Sprintf.
	T(message string, args ...any) string
	// E translate the message to the target language.
	// it works like fmt.Errorf.
	E(messgae string, args ...any) error
}

// Default is the default localizer.
var Default = &SimpleLocalizer{}

type SimpleLocalizer struct{}

func (s *SimpleLocalizer) T(message string, args ...any) string {
	return fmt.Sprintf(message, args...)
}

func (s *SimpleLocalizer) E(message string, args ...any) error {
	return fmt.Errorf(message, args...)
}
