package ws

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	corev1 "k8s.io/api/core/v1"
	httpstreamspdy "k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	kubectlscheme "k8s.io/kubectl/pkg/scheme"
	"xiaoshiai.cn/common/log"
)

func NewPodExecutRequest(cs kubernetes.Interface, namesapce, name string, options *corev1.PodExecOptions) *rest.Request {
	req := cs.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namesapce).
		Name(name).
		SubResource("exec").
		VersionedParams(options, kubectlscheme.ParameterCodec)
	return req
}

func NewPodExecutor(cs kubernetes.Interface, config *rest.Config,
	namespace, name string, options *corev1.PodExecOptions,
) (remotecommand.Executor, error) {
	req := NewPodExecutRequest(cs, namespace, name, options)
	return remotecommand.NewSPDYExecutor(config, "POST", req.URL())
}

func NewPodExecutorWithTransport(
	cs kubernetes.Interface, config *rest.Config,
	namespace, name string, options *corev1.PodExecOptions,
) (remotecommand.Executor, error) {
	upgradeRoundTripper, err := httpstreamspdy.NewRoundTripperWithConfig(httpstreamspdy.RoundTripperConfig{
		PingPeriod:       time.Second * 5,
		UpgradeTransport: config.Transport,
	})
	if err != nil {
		return nil, err
	}
	wrapper, err := rest.HTTPWrappersForConfig(config, upgradeRoundTripper)
	if err != nil {
		return nil, err
	}
	req := NewPodExecutRequest(cs, namespace, name, options)
	return remotecommand.NewSPDYExecutorForTransports(wrapper, upgradeRoundTripper, "POST", req.URL())
}

func NewWebSocketExecutor(cs kubernetes.Interface, config *rest.Config, namespace, name string, options *corev1.PodExecOptions) (remotecommand.Executor, error) {
	req := NewPodExecutRequest(cs, namespace, name, options)
	return remotecommand.NewWebSocketExecutor(config, "POST", req.URL().String())
}

func NewExecStream(ctx context.Context, ws *websocket.Conn) *StreamHandler {
	r, w := io.Pipe() // TODO: may use a cached pipe be better
	stream := &StreamHandler{
		Conn:        ws,
		r:           r,
		ResizeEvent: make(chan *remotecommand.TerminalSize, 1),
	}
	go func() {
		if err := stream.readLoop(ctx, w); err != nil {
			log.FromContext(ctx).Error(err, "websocket read loop error")
		}
	}()
	return stream
}

type StreamHandler struct {
	Conn        *websocket.Conn
	r           io.Reader
	Cache       []byte
	writelock   sync.Mutex
	ResizeEvent chan *remotecommand.TerminalSize
}

type xtermMessage struct {
	MsgType string `json:"type"`
	Input   string `json:"input"`
	Rows    uint16 `json:"rows"`
	Cols    uint16 `json:"cols"`
}

func (s *StreamHandler) Next() (size *remotecommand.TerminalSize) {
	return <-s.ResizeEvent
}

func (s *StreamHandler) Close() error {
	return s.Conn.Close()
}

func (s *StreamHandler) readLoop(ctx context.Context, w *io.PipeWriter) error {
	// close the writer when the function returns
	// it notifies the reader that the stream is closed
	defer w.Close()

	xmsg := xtermMessage{}
	leftover := []byte{}
	for {
		select {
		case <-ctx.Done():
		default:
			if len(leftover) > 0 {
				n, err := w.Write(leftover)
				if err != nil {
					return err
				}
				if n < len(leftover) {
					leftover = leftover[n:]
				} else {
					leftover = nil
				}
				continue
			}
			// newly read
			msgType, data, err := s.Conn.ReadMessage()
			if err != nil {
				return err
			}
			_ = msgType
			if e := json.Unmarshal(data, &xmsg); e != nil {
				// just ignore the message
				continue
			}
			switch xmsg.MsgType {
			case "resize":
				select {
				case s.ResizeEvent <- &remotecommand.TerminalSize{Width: xmsg.Cols, Height: xmsg.Rows}:
				case <-ctx.Done():
					return nil
				default:
				}
			case "input":
				data := []byte(xmsg.Input)
				n, err := w.Write(data)
				if err != nil {
					return err
				}
				if n < len(data) {
					leftover = data[n:]
				} else {
					leftover = nil
				}
			case "close":
				return nil
			}
		}
	}
}

func (s *StreamHandler) Read(p []byte) (size int, err error) {
	return s.r.Read(p)
}

func (s *StreamHandler) Write(p []byte) (size int, err error) {
	s.writelock.Lock()
	defer s.writelock.Unlock()
	if err = s.Conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}
