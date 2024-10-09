package log

import (
	"k8s.io/klog/v2"
)

var (
	NewContext  = klog.NewContext
	FromContext = klog.FromContext
)

func init() {
	klog.EnableContextualLogging(true)
}

var (
	Info  = DefaultLogger.Info
	Error = DefaultLogger.Error
	V     = DefaultLogger.V
	Warn  = DefaultLogger.V(1)
	Debug = DefaultLogger.V(2)
	Trace = DefaultLogger.V(3)
)

var DefaultLogger = klog.Background()

type Logger = klog.Logger

func Ref(namespace, name string) klog.ObjectRef {
	return klog.KRef(namespace, name)
}
