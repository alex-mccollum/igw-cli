package cli

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type rpcSessionState struct {
	mu       sync.Mutex
	inFlight map[string]context.CancelFunc
}

func newRPCSessionState() *rpcSessionState {
	return &rpcSessionState{
		inFlight: make(map[string]context.CancelFunc),
	}
}

func rpcRequestIDKey(id any) (string, bool) {
	if id == nil {
		return "", false
	}
	key := strings.TrimSpace(fmt.Sprint(id))
	if key == "" {
		return "", false
	}
	return key, true
}

func (s *rpcSessionState) registerInFlight(id any, cancel context.CancelFunc) (string, bool) {
	if s == nil || cancel == nil {
		return "", false
	}
	key, ok := rpcRequestIDKey(id)
	if !ok {
		return "", false
	}
	s.mu.Lock()
	s.inFlight[key] = cancel
	s.mu.Unlock()
	return key, true
}

func (s *rpcSessionState) unregisterInFlight(key string) {
	if s == nil || key == "" {
		return
	}
	s.mu.Lock()
	delete(s.inFlight, key)
	s.mu.Unlock()
}

func (s *rpcSessionState) cancelRequest(key string) bool {
	if s == nil || strings.TrimSpace(key) == "" {
		return false
	}
	s.mu.Lock()
	cancel, ok := s.inFlight[key]
	if ok {
		delete(s.inFlight, key)
	}
	s.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}
