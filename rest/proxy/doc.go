// Package proxy forwards requests to an upstream service and keeps the responses
// usable behind whatever path prefix this process exposes.
//
// Transport rewrites the URLs a service emits for itself -- Location headers and
// HTML URL attributes -- back onto the prefix the service was reached on; any
// other response content passes through untouched.
//
// transport.go is derived from k8s.io/apimachinery/pkg/util/proxy/transport.go
// (Apache-2.0, Copyright 2014 The Kubernetes Authors). The copy still matches
// v0.35.2: v0.36 moved that file to structured logging (klog.FromContext) and
// that change has not been taken. Apart from PathRemove and RewritePath this
// package intends to stay textually identical to upstream, so re-syncing is a
// diff review rather than a rewrite.
//
// The local differences from upstream are:
//
//   - PathRemove, which upstream does not have. Upstream can only prepend the
//     external prefix to a path the service already reports in external terms,
//     so it cannot express a route whose upstream path differs from its external
//     one (/api/moha reaching /v1/). Every service-exposure route rewrites this
//     way.
//   - RewritePath, which replaces upstream's path.Join. path.Join collapses
//     repeated separators, re-reads an escaped %2F as a separator, removes dot
//     segments, and drops the original escapes. RewritePath keeps the suffix
//     byte for byte instead, which repository and object names rely on. It is
//     exported so callers rewriting an outbound path themselves use the same
//     rules.
//   - The "already prefixed" test in rewriteURL matches on a segment boundary.
//     Upstream compares string prefixes, so it also matches a sibling path and
//     would mistake /api/airouter-other for /api/airouter.
//
// Transport.Scheme and Transport.Host are the client-facing origin, not the
// upstream address, and the caller supplies them. Upstream derives them from the
// request URL, which is empty on the server side; here they normally come from
// the forwarded headers. With both empty, rewritten URLs stay relative. A
// service that answers with an absolute URL naming its own authority is
// rewritten onto that origin; any other authority passes through unchanged, so a
// proxied service must not report an address it was not reached on.
//
// proxy.go is not derived from upstream: it composes Transport with the
// platform's client configuration and the X-Forwarded-Uri prefix convention.
package proxy
