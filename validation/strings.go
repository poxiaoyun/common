package validation

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxURLRuneCount = 2083
	minURLRuneCount = 3
)

const RF3339WithoutZone = "2006-01-02T15:04:05"

// nolint lll
const (
	Email                string = "^(((([a-zA-Z]|\\d|[!#\\$%&'\\*\\+\\-\\/=\\?\\^_`{\\|}~]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])+(\\.([a-zA-Z]|\\d|[!#\\$%&'\\*\\+\\-\\/=\\?\\^_`{\\|}~]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])+)*)|((\\x22)((((\\x20|\\x09)*(\\x0d\\x0a))?(\\x20|\\x09)+)?(([\\x01-\\x08\\x0b\\x0c\\x0e-\\x1f\\x7f]|\\x21|[\\x23-\\x5b]|[\\x5d-\\x7e]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])|(\\([\\x01-\\x09\\x0b\\x0c\\x0d-\\x7f]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}]))))*(((\\x20|\\x09)*(\\x0d\\x0a))?(\\x20|\\x09)+)?(\\x22)))@((([a-zA-Z]|\\d|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])|(([a-zA-Z]|\\d|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])([a-zA-Z]|\\d|-|\\.|_|~|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])*([a-zA-Z]|\\d|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])))\\.)+(([a-zA-Z]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])|(([a-zA-Z]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])([a-zA-Z]|\\d|-|_|~|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])*([a-zA-Z]|[\\x{00A0}-\\x{D7FF}\\x{F900}-\\x{FDCF}\\x{FDF0}-\\x{FFEF}])))\\.?$"
	URLSchema            string = `((ftp|tcp|udp|wss?|https?):\/\/)`
	URLUsername          string = `(\S+(:\S*)?@)`
	URLPath              string = `((\/|\?|#)[^\s]*)`
	IP                   string = `(([0-9a-fA-F]{1,4}:){7,7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:)|fe80:(:[0-9a-fA-F]{0,4}){0,4}%[0-9a-zA-Z]{1,}|::(ffff(:0{1,4}){0,1}:){0,1}((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])|([0-9a-fA-F]{1,4}:){1,4}:((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9]))`
	URLPort              string = `(:(\d{1,5}))`
	URLIP                string = `([1-9]\d?|1\d\d|2[01]\d|22[0-3]|24\d|25[0-5])(\.(\d{1,2}|1\d\d|2[0-4]\d|25[0-5])){2}(?:\.([0-9]\d?|1\d\d|2[0-4]\d|25[0-5]))`
	URLSubdomain         string = `((www\.)|([a-zA-Z0-9]+([-_\.]?[a-zA-Z0-9])*[a-zA-Z0-9]\.[a-zA-Z0-9]+))`
	URL                  string = `^` + URLSchema + `?` + URLUsername + `?` + `((` + URLIP + `|(\[` + IP + `\])|(([a-zA-Z0-9]([a-zA-Z0-9-_]+)?[a-zA-Z0-9]([-\.][a-zA-Z0-9]+)*)|(` + URLSubdomain + `?))?(([a-zA-Z\x{00a1}-\x{ffff}0-9]+-?-?)*[a-zA-Z\x{00a1}-\x{ffff}0-9]+)(?:\.([a-zA-Z\x{00a1}-\x{ffff}]{1,}))?))\.?` + URLPort + `?` + URLPath + `?$`
	Alpha                string = "^[a-zA-Z]+$"
	Alphanumeric         string = "^[a-zA-Z0-9]+$"
	Numeric              string = "^[0-9]+$"
	Int                  string = "^(?:[-+]?(?:0|[1-9][0-9]*))$"
	Float                string = "^(?:[-+]?(?:[0-9]+))?(?:\\.[0-9]*)?(?:[eE][\\+\\-]?(?:[0-9]+))?$"
	hasWhitespace        string = ".*[[:space:]]"
	hasWhitespaceOnly    string = "^[[:space:]]+$"
	Base64               string = "^(?:[A-Za-z0-9+\\/]{4})*(?:[A-Za-z0-9+\\/]{2}==|[A-Za-z0-9+\\/]{3}=|[A-Za-z0-9+\\/]{4})$"
	WinPath              string = `^[a-zA-Z]:\\(?:[^\\/:*?"<>|\r\n]+\\)*[^\\/:*?"<>|\r\n]*$`
	UnixPath             string = `^(/[^/\x00]*)+/?$`
	DataURI              string = "^data:.+\\/(.+);base64$"
	DNSName              string = `^([a-zA-Z0-9_]{1}[a-zA-Z0-9_-]{0,62}){1}(\.[a-zA-Z0-9_]{1}[a-zA-Z0-9_-]{0,62})*[\._]?$`
	Semver               string = "^v?(?:0|[1-9]\\d*)\\.(?:0|[1-9]\\d*)\\.(?:0|[1-9]\\d*)(-(0|[1-9]\\d*|\\d*[a-zA-Z-][0-9a-zA-Z-]*)(\\.(0|[1-9]\\d*|\\d*[a-zA-Z-][0-9a-zA-Z-]*))*)?(\\+[0-9a-zA-Z-]+(\\.[0-9a-zA-Z-]+)*)?$"
	ResourcesNamePattern string = "^[a-zA-Z][a-zA-Z0-9-]{0,254}$"
	LabelKeyValueRegexp  string = "^[a-zA-Z0-9_.-]{0,255}$"
	EnvNameRegexp        string = "^[a-zA-Z_]+[a-zA-Z0-9_]*$"
)

var (
	rxEmail             = regexp.MustCompile(Email)
	rxURL               = regexp.MustCompile(URL)
	rxAlpha             = regexp.MustCompile(Alpha)
	rxAlphanumeric      = regexp.MustCompile(Alphanumeric)
	rxNumeric           = regexp.MustCompile(Numeric)
	rxHasWhitespace     = regexp.MustCompile(hasWhitespace)
	rxHasWhitespaceOnly = regexp.MustCompile(hasWhitespaceOnly)
	rxBase64            = regexp.MustCompile(Base64)
	rxWinPath           = regexp.MustCompile(WinPath)
	rxUnixPath          = regexp.MustCompile(UnixPath)
	rxDataURI           = regexp.MustCompile(DataURI)
	rxDNSName           = regexp.MustCompile(DNSName)
	rxSemver            = regexp.MustCompile(Semver)
	rxInt               = regexp.MustCompile(Int)
	rxFloat             = regexp.MustCompile(Float)
	rxResourcesName     = regexp.MustCompile(ResourcesNamePattern)
)

func IsURL(str string) bool {
	if str == "" ||
		utf8.RuneCountInString(str) >= maxURLRuneCount ||
		len(str) <= minURLRuneCount ||
		strings.HasPrefix(str, ".") {
		return false
	}
	strTemp := str
	if strings.Contains(str, ":") && !strings.Contains(str, "://") {
		// support no indicated urlscheme but with colon for port number
		// http:// is appended so url.Parse will succeed, strTemp used so it does not impact rxURL.MatchString
		strTemp = "http://" + str
	}
	u, err := url.Parse(strTemp)
	if err != nil {
		return false
	}
	if strings.HasPrefix(u.Host, ".") {
		return false
	}
	if u.Host == "" && (u.Path != "" && !strings.Contains(u.Path, ".")) {
		return false
	}
	return rxURL.MatchString(str)
}

// IsEmail check if the string is an email.
func IsEmail(str string) bool {
	// TODO uppercase letters are not supported
	return rxEmail.MatchString(str)
}

// IsAlpha check if the string contains only letters (a-zA-Z). Empty string is valid.
func IsAlpha(str string) bool {
	if IsNull(str) {
		return true
	}
	return rxAlpha.MatchString(str)
}

// IsNumeric check if the string contains only numbers. Empty string is valid.
func IsNumeric(str string) bool {
	if IsNull(str) {
		return true
	}
	return rxNumeric.MatchString(str)
}

// IsAlphanumeric check if the string contains only letters and numbers. Empty string is valid.
func IsAlphanumeric(str string) bool {
	if IsNull(str) {
		return true
	}
	return rxAlphanumeric.MatchString(str)
}

// IsNotNull check if the string is not null.
func IsNotNull(str string) bool {
	return !IsNull(str)
}

// HasWhitespaceOnly checks the string only contains whitespace
func HasWhitespaceOnly(str string) bool {
	return len(str) > 0 && rxHasWhitespaceOnly.MatchString(str)
}

// HasWhitespace checks if the string contains any whitespace
func HasWhitespace(str string) bool {
	return len(str) > 0 && rxHasWhitespace.MatchString(str)
}

// IsBase64 check if a string is base64 encoded.
func IsBase64(str string) bool {
	return rxBase64.MatchString(str)
}

const MaxFileLen = 32767

// IsFilePath check is a string is Win or Unix file path and returns it's type(windows,unix,unknown).
func IsFilePath(str string) (bool, string) {
	switch {
	case rxWinPath.MatchString(str):
		// check windows path limit see:
		//  http://msdn.microsoft.com/en-us/library/aa365247(VS.85).aspx#maxpath
		if len(str[3:]) > MaxFileLen {
			return false, "windows"
		}
		return true, "windows"
	case rxUnixPath.MatchString(str):
		return true, "unix"
	default:
		return false, "unknown"
	}
}

// IsDataURI checks if a string is base64 encoded data URI such as an image
func IsDataURI(str string) bool {
	dataURI := strings.Split(str, ",")
	if !rxDataURI.MatchString(dataURI[0]) {
		return false
	}
	return IsBase64(dataURI[1])
}

// IsDNSName will validate the given string as a DNS name
func IsDNSName(str string) bool {
	if str == "" || len(strings.ReplaceAll(str, ".", "")) > 255 {
		// constraints already violated
		return false
	}
	return !IsIP(str) && rxDNSName.MatchString(str)
}

// IsIP checks if a string is either IP version 4 or 6. Alias for `net.ParseIP`
func IsIP(str string) bool {
	return net.ParseIP(str) != nil
}

// IsPort checks if a string represents a valid port
func IsPort(str string) bool {
	if i, err := strconv.Atoi(str); err == nil && i > 0 && i < 65536 {
		return true
	}
	return false
}

// IsIPv4 check if the string is an IP version 4.
func IsIPv4(str string) bool {
	ip := net.ParseIP(str)
	return ip != nil && strings.Contains(str, ".")
}

// IsIPv6 check if the string is an IP version 6.
func IsIPv6(str string) bool {
	ip := net.ParseIP(str)
	return ip != nil && strings.Contains(str, ":")
}

// IsCIDR check if the string is an valid CIDR notiation (IPV4 & IPV6)
func IsCIDR(str string) bool {
	_, _, err := net.ParseCIDR(str)
	return err == nil
}

// IsHost checks if the string is a valid IP (both v4 and v6) or a valid DNS name
func IsHost(str string) bool {
	return IsIP(str) || IsDNSName(str)
}

// IsSemver check if string is valid semantic version
func IsSemver(str string) bool {
	return rxSemver.MatchString(str)
}

// IsTime check if string is valid according to given format
func IsTime(str string, format string) bool {
	_, err := time.Parse(format, str)
	return err == nil
}

// IsUnixTime check if string is valid unix timestamp value
func IsUnixTime(str string) bool {
	if _, err := strconv.Atoi(str); err == nil {
		return true
	}
	return false
}

// IsRFC3339 check if string is valid timestamp value according to RFC3339
func IsRFC3339(str string) bool {
	return IsTime(str, time.RFC3339)
}

// IsRFC3339WithoutZone check if string is valid timestamp value according to RFC3339 which excludes the timezone.
func IsRFC3339WithoutZone(str string) bool {
	return IsTime(str, RF3339WithoutZone)
}

const ByteToBitLen = 8

// IsRsaPublicKey check if a string is valid public key with provided length
func IsRsaPublicKey(str string, keylen int) bool {
	block, _ := pem.Decode([]byte(str))
	if block != nil && block.Type != "PUBLIC KEY" {
		return false
	}
	var der []byte
	var err error

	if block != nil {
		der = block.Bytes
	} else {
		der, err = base64.StdEncoding.DecodeString(str)
		if err != nil {
			return false
		}
	}

	key, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return false
	}
	pubkey, ok := key.(*rsa.PublicKey)
	if !ok {
		return false
	}
	bitlen := len(pubkey.N.Bytes()) * ByteToBitLen
	return bitlen == keylen
}

// StringMatches checks if a string matches a given pattern.
func StringMatches(s string, params ...string) bool {
	if len(params) == 1 {
		pattern := params[0]
		return Matches(s, pattern)
	}
	return false
}

// Matches checks if string matches the pattern (pattern is regular expression)
// In case of error return false
func Matches(str, pattern string) bool {
	match, _ := regexp.MatchString(pattern, str)
	return match
}

// IsInt check if the string is an integer. Empty string is valid.
func IsInt(str string) bool {
	if IsNull(str) {
		return true
	}
	return rxInt.MatchString(str)
}

// IsFloat check if the string is a float.
func IsFloat(str string) bool {
	return str != "" && rxFloat.MatchString(str)
}

// IsNull check if the string is null.
func IsNull(str string) bool {
	return len(str) == 0
}

func IsResourcesName(str string) bool {
	return rxResourcesName.MatchString(str)
}
