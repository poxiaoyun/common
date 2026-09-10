package api

import (
	"cmp"
	"fmt"
	"maps"
	"mime"
	"net/http"
	"slices"
	"strings"
)

type mediaRange struct {
	typeName string
	subtype  string
	params   map[string]string
}

type mediaSpecificity struct {
	kind   int
	params int
}

func (s mediaSpecificity) compare(other mediaSpecificity) int {
	if order := cmp.Compare(s.kind, other.kind); order != 0 {
		return order
	}
	return cmp.Compare(s.params, other.params)
}

func (m mediaRange) specificity() mediaSpecificity {
	kind := 2
	if m.typeName == "*" {
		kind = 0
	} else if m.subtype == "*" {
		kind = 1
	}
	return mediaSpecificity{kind: kind, params: len(m.params)}
}

func parseMediaRange(value string) (mediaRange, error) {
	name, params, err := mime.ParseMediaType(value)
	if err != nil {
		return mediaRange{}, err
	}
	typeName, subtype, ok := strings.Cut(name, "/")
	if !ok || typeName == "" || subtype == "" || (typeName == "*" && subtype != "*") {
		return mediaRange{}, fmt.Errorf("invalid media range %q", value)
	}
	if charset, ok := params["charset"]; ok {
		params["charset"] = strings.ToLower(charset)
	}
	return mediaRange{typeName: typeName, subtype: subtype, params: params}, nil
}

func compileMediaRanges(values []string) ([]mediaRange, []string, error) {
	var ranges []mediaRange
	var normalized []string
	for _, value := range values {
		media, err := parseMediaRange(value)
		if err != nil {
			return nil, nil, err
		}
		if _, ok := media.params["q"]; ok {
			return nil, nil, fmt.Errorf("media condition %q cannot declare an Accept quality", value)
		}
		name := mime.FormatMediaType(media.typeName+"/"+media.subtype, media.params)
		if !slices.Contains(normalized, name) {
			ranges = append(ranges, media)
			normalized = append(normalized, name)
		}
	}
	return ranges, normalized, nil
}

func (m mediaRange) contains(other mediaRange) bool {
	if m.typeName != "*" && m.typeName != other.typeName {
		return false
	}
	if m.subtype != "*" && m.subtype != other.subtype {
		return false
	}
	for key, value := range m.params {
		if actual, ok := other.params[key]; !ok || actual != value {
			return false
		}
	}
	return true
}

func intersectMediaRange(a, b mediaRange) (mediaRange, bool) {
	result := a
	if a.typeName == "*" {
		result.typeName = b.typeName
	} else if b.typeName != "*" && a.typeName != b.typeName {
		return mediaRange{}, false
	}
	if a.subtype == "*" {
		result.subtype = b.subtype
	} else if b.subtype != "*" && a.subtype != b.subtype {
		return mediaRange{}, false
	}
	result.params = maps.Clone(a.params)
	for key, value := range b.params {
		if existing, ok := result.params[key]; ok && existing != value {
			return mediaRange{}, false
		}
		if result.params == nil {
			result.params = map[string]string{}
		}
		result.params[key] = value
	}
	return result, true
}

func intersectMediaConditions(parent, child []string) ([]string, error) {
	parents, normalizedParent, err := compileMediaRanges(parent)
	if err != nil {
		return nil, err
	}
	children, normalizedChild, err := compileMediaRanges(child)
	if err != nil {
		return nil, err
	}
	if len(parents) == 0 {
		return normalizedChild, nil
	}
	if len(children) == 0 {
		return normalizedParent, nil
	}
	var result []string
	for _, a := range parents {
		for _, b := range children {
			if intersection, ok := intersectMediaRange(a, b); ok {
				name := mime.FormatMediaType(intersection.typeName+"/"+intersection.subtype, intersection.params)
				if !slices.Contains(result, name) {
					result = append(result, name)
				}
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("media conditions %v and %v have no intersection", parent, child)
	}
	return result, nil
}

type acceptRange struct {
	mediaRange
	quality int
}

type requestMedia struct {
	contentType      mediaRange
	validContentType bool
	accept           []acceptRange
}

func parseRequestMedia(header http.Header) requestMedia {
	request := requestMedia{}
	if values := header.Values("Content-Type"); len(values) == 1 {
		contentType, err := parseMediaRange(values[0])
		if err != nil {
			request.validContentType = false
		} else {
			request.contentType = contentType
			request.validContentType = contentType.typeName != "*" && contentType.subtype != "*"
		}
	}
	values := header.Values("Accept")
	if len(values) == 0 {
		request.accept = []acceptRange{{mediaRange: mediaRange{typeName: "*", subtype: "*"}, quality: 1000}}
		return request
	}
	parts, ok := splitMediaList(strings.Join(values, ","))
	if !ok {
		return request
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		media, err := parseMediaRange(part)
		if err != nil {
			request.accept = nil
			return request
		}
		quality := 1000
		if value, ok := media.params["q"]; ok {
			quality, ok = parseQuality(value)
			if !ok {
				request.accept = nil
				return request
			}
			delete(media.params, "q")
		}
		request.accept = append(request.accept, acceptRange{mediaRange: media, quality: quality})
	}
	return request
}

// Accept uses an HTTP list; quoted parameter values can contain commas.
func splitMediaList(value string) ([]string, bool) {
	var parts []string
	start, quoted, escaped := 0, false, false
	for index, char := range value {
		if escaped {
			escaped = false
			continue
		}
		switch char {
		case '\\':
			escaped = quoted
		case '"':
			quoted = !quoted
		case ',':
			if !quoted {
				parts = append(parts, value[start:index])
				start = index + 1
			}
		}
	}
	return append(parts, value[start:]), !quoted && !escaped
}

func parseQuality(value string) (int, bool) {
	whole, fraction, dotted := strings.Cut(value, ".")
	if (whole != "0" && whole != "1") || len(fraction) > 3 {
		return 0, false
	}
	quality := 0
	for _, digit := range fraction {
		if digit < '0' || digit > '9' || (whole == "1" && digit != '0') {
			return 0, false
		}
		quality = quality*10 + int(digit-'0')
	}
	if whole == "1" {
		return 1000, true
	}
	if dotted {
		for index := len(fraction); index < 3; index++ {
			quality *= 10
		}
	}
	return quality, true
}

type mediaMatch struct {
	contentType mediaSpecificity
	hasAccept   bool
	quality     int
	accept      mediaSpecificity
	offered     mediaSpecificity
}

func (m mediaMatch) compare(other mediaMatch) int {
	if order := m.contentType.compare(other.contentType); order != 0 {
		return order
	}
	if m.hasAccept != other.hasAccept {
		if m.hasAccept {
			return 1
		}
		return -1
	}
	if order := cmp.Compare(m.quality, other.quality); order != 0 {
		return order
	}
	if order := m.accept.compare(other.accept); order != 0 {
		return order
	}
	return m.offered.compare(other.offered)
}

func (r requestMedia) match(contentTypes, accepts []mediaRange) (mediaMatch, bool) {
	result := mediaMatch{contentType: mediaSpecificity{kind: -1}, quality: 1000}
	if len(contentTypes) > 0 {
		if !r.validContentType {
			return mediaMatch{}, false
		}
		for _, media := range contentTypes {
			specificity := media.specificity()
			if media.contains(r.contentType) && specificity.compare(result.contentType) > 0 {
				result.contentType = specificity
			}
		}
		if result.contentType.kind == -1 {
			return mediaMatch{}, false
		}
	}
	if len(accepts) == 0 {
		return result, true
	}
	result.hasAccept, result.quality = true, 0
	for _, offered := range accepts {
		for _, accepted := range r.accept {
			intersection, ok := intersectMediaRange(offered, accepted.mediaRange)
			if !ok || !maps.Equal(intersection.params, offered.params) {
				continue
			}
			// Select the most specific range before reading its quality. A
			// concrete q=0 exclusion must override a positive wildcard.
			quality, specificity := 0, mediaSpecificity{kind: -1}
			for _, candidate := range r.accept {
				precedence := candidate.specificity()
				if candidate.contains(intersection) && precedence.compare(specificity) > 0 {
					quality, specificity = candidate.quality, precedence
				}
			}
			match := result
			match.quality, match.accept, match.offered = quality, specificity, offered.specificity()
			if match.compare(result) > 0 {
				result = match
			}
		}
	}
	return result, result.quality > 0
}
