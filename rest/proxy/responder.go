package proxy

import (
	"errors"
	"net"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	liberrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
	"xiaoshiai.cn/common/rest/api"
)

type ErrorResponser struct{}

func (ErrorResponser) Error(w http.ResponseWriter, req *http.Request, err error) {
	dnse := &net.DNSError{}
	if errors.As(err, &dnse) {
		err = liberrors.NewServiceUnavailable(err.Error())
	}
	statuserr := &apierrors.StatusError{}
	if errors.As(err, &statuserr) {
		api.Error(w, liberrors.NewCustomError(
			int(statuserr.ErrStatus.Code),
			liberrors.StatusReason(statuserr.ErrStatus.Reason),
			statuserr.ErrStatus.Message))
		return
	}
	api.Error(w, err)
	log.Error(err, "proxy error", "url", req.URL.String())
}
