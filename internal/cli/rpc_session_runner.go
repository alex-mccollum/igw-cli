package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

type rpcWorkItem struct {
	req        rpcRequest
	enqueuedAt time.Time
}

type rpcSessionRunner struct {
	cli       *CLI
	common    wrapperCommon
	specFile  string
	workers   int
	queueSize int
}

func (r *rpcSessionRunner) run() error {
	scanner := bufio.NewScanner(r.cli.In)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	workQueue := make(chan rpcWorkItem, r.queueSize)
	results := make(chan rpcResponse, r.queueSize)
	session := newRPCSessionState()

	var workerWG sync.WaitGroup
	r.startWorkers(session, workQueue, results, &workerWG)

	writeErrCh := r.startResponseWriter(results)
	scanErr, stopRead := r.scanRequests(scanner, workQueue, results)

	close(workQueue)
	workerWG.Wait()
	close(results)

	if writeErr := <-writeErrCh; writeErr != nil {
		return writeErr
	}
	if scanErr != nil {
		return scanErr
	}
	if stopRead {
		return nil
	}
	return nil
}

func (r *rpcSessionRunner) startWorkers(session *rpcSessionState, workQueue <-chan rpcWorkItem, results chan<- rpcResponse, workerWG *sync.WaitGroup) {
	for worker := 0; worker < r.workers; worker++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for work := range workQueue {
				queueWaitMs := time.Since(work.enqueuedAt).Milliseconds()
				queueDepth := len(workQueue)
				resp := r.cli.handleRPCRequest(work.req, r.common, r.specFile, session)
				if strings.EqualFold(strings.TrimSpace(work.req.Op), "call") {
					resp = withRPCCallQueueStats(resp, queueWaitMs, queueDepth)
				}
				results <- resp
			}
		}()
	}
}

func (r *rpcSessionRunner) startResponseWriter(results <-chan rpcResponse) <-chan error {
	writeErrCh := make(chan error, 1)
	go func() {
		enc := json.NewEncoder(r.cli.Out)
		enc.SetEscapeHTML(false)
		var writeErr error
		for result := range results {
			if writeErr == nil {
				if err := enc.Encode(result); err != nil {
					writeErr = igwerr.NewTransportError(err)
				}
			}
		}
		writeErrCh <- writeErr
	}()
	return writeErrCh
}

func (r *rpcSessionRunner) scanRequests(scanner *bufio.Scanner, workQueue chan<- rpcWorkItem, results chan<- rpcResponse) (error, bool) {
	stopRead := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			results <- rpcResponse{
				OK:    false,
				Code:  igwerr.ExitCode(&igwerr.UsageError{Msg: "invalid rpc request json"}),
				Error: fmt.Sprintf("invalid rpc request json: %v", err),
			}
			continue
		}

		workQueue <- rpcWorkItem{req: req, enqueuedAt: time.Now()}
		if strings.EqualFold(strings.TrimSpace(req.Op), "shutdown") {
			stopRead = true
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return igwerr.NewTransportError(err), stopRead
	}
	return nil, stopRead
}
