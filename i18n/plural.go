package i18n

// DefaultPluralRules returns the default plural rules for common languages.
func DefaultPluralRules() map[string]PluralRule {
	return map[string]PluralRule{
		// English
		"en": func(n int) string {
			if n == 1 {
				return PluralFormOne
			}
			return PluralFormOther
		},

		// Chinese (Simplified and Traditional) - no plural forms
		"zh":    func(n int) string { return PluralFormOther },
		"zh-CN": func(n int) string { return PluralFormOther },
		"zh-TW": func(n int) string { return PluralFormOther },

		// Japanese - no plural forms
		"ja": func(n int) string { return PluralFormOther },

		// Korean - no plural forms
		"ko": func(n int) string { return PluralFormOther },

		// Spanish, Italian, Portuguese
		"es": func(n int) string {
			if n == 1 {
				return PluralFormOne
			}
			return PluralFormOther
		},
		"it": func(n int) string {
			if n == 1 {
				return PluralFormOne
			}
			return PluralFormOther
		},
		"pt": func(n int) string {
			if n == 1 {
				return PluralFormOne
			}
			return PluralFormOther
		},

		// French
		"fr": func(n int) string {
			if n == 0 || n == 1 {
				return PluralFormOne
			}
			return PluralFormOther
		},

		// German
		"de": func(n int) string {
			if n == 1 {
				return PluralFormOne
			}
			return PluralFormOther
		},

		// Russian - complex plural rules (3 forms)
		"ru": func(n int) string {
			mod10 := n % 10
			mod100 := n % 100

			if mod10 == 1 && mod100 != 11 {
				return PluralFormOne
			}
			if mod10 >= 2 && mod10 <= 4 && (mod100 < 10 || mod100 >= 20) {
				return PluralFormFew
			}
			return PluralFormMany
		},

		// Polish - complex plural rules (3 forms)
		"pl": func(n int) string {
			mod10 := n % 10
			mod100 := n % 100

			if n == 1 {
				return PluralFormOne
			}
			if mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14) {
				return PluralFormFew
			}
			return PluralFormMany
		},

		// Arabic - complex plural rules (6 forms)
		"ar": func(n int) string {
			mod100 := n % 100

			if n == 0 {
				return PluralFormZero
			}
			if n == 1 {
				return PluralFormOne
			}
			if n == 2 {
				return PluralFormTwo
			}
			if mod100 >= 3 && mod100 <= 10 {
				return PluralFormFew
			}
			if mod100 >= 11 && mod100 <= 99 {
				return PluralFormMany
			}
			return PluralFormOther
		},

		// Czech, Slovak - complex plural rules (3 forms)
		"cs": func(n int) string {
			if n == 1 {
				return PluralFormOne
			}
			if n >= 2 && n <= 4 {
				return PluralFormFew
			}
			return PluralFormOther
		},
		"sk": func(n int) string {
			if n == 1 {
				return PluralFormOne
			}
			if n >= 2 && n <= 4 {
				return PluralFormFew
			}
			return PluralFormOther
		},
	}
}

// AddPluralRule adds or overrides a plural rule for a language.
func (m *manager) AddPluralRule(lang string, rule PluralRule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pluralRules[lang] = rule
}
