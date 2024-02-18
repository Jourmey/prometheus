package scrape

import (
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"github.com/prometheus/prometheus/util/clusterpb"
	"github.com/prometheus/prometheus/util/proto"
	"io"
	"net/http"
	"strings"
	"time"
)

type pomeloTargetScraper struct {
	*Target

	client *http.Client
	//req     *http.Request
	timeout time.Duration

	gzipr *gzip.Reader
	buf   *bufio.Reader

	bodySizeLimit int64
	acceptHeader  string

	agent clusterpb.MemberClientAgent
	req   *proto.RequestRequest
}

func (s *pomeloTargetScraper) scrape(ctx context.Context, w io.Writer) (string, error) {

	//	var (
	//		txt = `# HELP go_gc_duration_seconds A summary of the pause duration of garbage collection cycles.
	//# TYPE go_gc_duration_seconds summary
	//go_gc_duration_seconds{quantile="0"} 4.2383e-05
	//go_gc_duration_seconds{quantile="0.25"} 5.1827e-05
	//go_gc_duration_seconds{quantile="0.5"} 5.8333e-05
	//go_gc_duration_seconds{quantile="0.75"} 6.9522e-05
	//go_gc_duration_seconds{quantile="1"} 0.000420957
	//go_gc_duration_seconds_sum 0.02083746
	//go_gc_duration_seconds_count 309
	//`
	//	)
	//
	//	_, err := w.Write([]byte(txt))
	//	if err != nil {
	//		return "", err
	//	}
	//

	if s.agent == nil {

		url := s.URL()

		paths := strings.Split(strings.TrimPrefix(url.Path, "/"), "/")
		if len(paths) != 4 {
			return "", errors.New("invalid pomelo path")
		}

		req := proto.RequestRequest{
			Namespace:  paths[0],
			ServerType: paths[1],
			Service:    paths[2],
			Method:     paths[3],
			Args:       nil,
		}

		s.req = &req

		agent := clusterpb.NewMqttMemberClient(url.Host)
		err := agent.Connect()
		if err != nil {
			return "", err
		}

		s.agent = agent
	}

	res, err := s.agent.Request(ctx, *s.req)
	if err != nil {
		return "", err
	}

	_ = res

	return "text/plain; version=0.0.4; charset=utf-8", nil
}
