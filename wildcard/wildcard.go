package wildcard

const (
	Star       = "*"  // match this section
	DoubleStar = "**" // match this section and all following sections
)

// acting like: https://shiro.apache.org/permissions.html#WildcardPermissions
// every section is separated by ':'
// * match zero or more characters at the same level
// ** match zero or more characters at any level
func Match(expr string, test string) bool {
	pi, pj := 0, 0

	for i, t := range test {
		switch {
		case t == ':':
			goto find
		case i == len(test)-1:
			i++
			goto find
		default:
			continue
		}
	find:
		matched := false
		for j := pj; j < len(expr); j++ {
			switch {
			case expr[j] == ':', expr[j] == ',':
				goto check
			case j == len(expr)-1:
				j++
				goto check
			default:
				continue
			}
		check:
			if expr[pj:j] == DoubleStar {
				return true
			}
			if expr[pj:j] == Star || expr[pj:j] == test[pi:i] {
				matched = true
			}
			pj = j + 1
			if j == len(expr) || expr[j] == ':' {
				break
			}
		}
		if !matched {
			return false
		}
		pi = i + 1
	}
	// test has fewer sections than expr
	for j := pj; j < len(expr); j++ {
		switch {
		case expr[j] == ':', expr[j] == ',':
			goto last
		case j == len(expr)-1:
			j++
			goto last
		default:
			continue
		}
	last:
		if expr[pj:j] == DoubleStar {
			return true
		}
		// tom:* will matches tom: tom if enabled
		if false && expr[pj:j] == Star {
			continue
		}
		return false
	}
	return true
}
