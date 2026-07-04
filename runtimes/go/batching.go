package runtime

import (
	"context"
	"fmt"
	"time"
)

type BatchHandler func(ctx context.Context, reqs []interface{}) ([]interface{}, error)

type BatchScheduler struct {
	maxBatchSize int
	waitWindow   time.Duration
	reqCh        chan *batchRequest
}

type batchRequest struct {
	ctx    context.Context
	req    interface{}
	respCh chan batchResponse
}

type batchResponse struct {
	resp interface{}
	err  error
}

func NewBatchScheduler(maxBatchSize int, waitWindow time.Duration, handler BatchHandler) *BatchScheduler {
	s := &BatchScheduler{
		maxBatchSize: maxBatchSize,
		waitWindow:   waitWindow,
		reqCh:        make(chan *batchRequest, maxBatchSize*100),
	}
	go s.run(handler)
	return s
}

func (s *BatchScheduler) run(handler BatchHandler) {
	var batch []*batchRequest
	var timer *time.Timer

	flush := func() {
		if len(batch) == 0 {
			return
		}

		reqs := make([]interface{}, len(batch))
		for i, br := range batch {
			reqs[i] = br.req
		}

		resps, err := handler(context.Background(), reqs)

		if err != nil {
			for _, br := range batch {
				br.respCh <- batchResponse{err: err}
			}
		} else if len(resps) != len(batch) {
			for _, br := range batch {
				br.respCh <- batchResponse{err: fmt.Errorf("batch handler returned %d responses for %d requests", len(resps), len(batch))}
			}
		} else {
			for i, br := range batch {
				br.respCh <- batchResponse{resp: resps[i]}
			}
		}

		batch = nil
		if timer != nil {
			timer.Stop()
			timer = nil
		}
	}

	for {
		if len(batch) == 0 {
			req := <-s.reqCh
			batch = append(batch, req)
			timer = time.NewTimer(s.waitWindow)
		}

		if len(batch) >= s.maxBatchSize {
			flush()
			continue
		}

		select {
		case req := <-s.reqCh:
			batch = append(batch, req)
			if len(batch) >= s.maxBatchSize {
				flush()
			}
		case <-timer.C:
			flush()
		}
	}
}

func (s *BatchScheduler) Invoke(ctx context.Context, req interface{}) (interface{}, error) {
	respCh := make(chan batchResponse, 1)
	br := &batchRequest{
		ctx:    ctx,
		req:    req,
		respCh: respCh,
	}
	s.reqCh <- br

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-respCh:
		return res.resp, res.err
	}
}

func (s *Server) RegisterBatchMethod(path string, maxBatchSize int, waitWindow time.Duration, methodInfo MethodInfo, handler BatchHandler) {
	scheduler := NewBatchScheduler(maxBatchSize, waitWindow, handler)
	methodInfo.Handler = scheduler.Invoke
	s.RegisterMethod(path, methodInfo)
}
