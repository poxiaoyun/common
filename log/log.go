package log

import (
	"strconv"

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

// SetVerbosity updates klog's process-wide V level.
func SetVerbosity(level int) error {
	verbosity := klog.Level(0)
	return verbosity.Set(strconv.Itoa(level))
}

type Logger = klog.Logger

func Ref(namespace, name string) klog.ObjectRef {
	return klog.KRef(namespace, name)
}
