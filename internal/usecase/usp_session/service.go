package usp_session

import (
	"acs/pkg/usp"
	"context"
	"fmt"
	"log/slog"
	"sync"
)

type Connection interface {
	Send(msg *usp.Msg) error
	Close() error
}

type Service struct {
	logger *slog.Logger
	mu     sync.RWMutex
	conns  map[string]Connection // EndpointID -> Connection
}

func NewService(logger *slog.Logger) *Service {
	return &Service{
		logger: logger,
		conns:  make(map[string]Connection),
	}
}

func (s *Service) HandleConnect(ctx context.Context, endpointID string, conn Connection) error {
	s.mu.Lock()
	if old, exists := s.conns[endpointID]; exists {
		old.Close()
	}
	s.conns[endpointID] = conn
	s.mu.Unlock()

	s.logger.Info("USP Endpoint connected", "endpoint_id", endpointID)
	return nil
}

func (s *Service) HandleDisconnect(endpointID string) {
	s.mu.Lock()
	delete(s.conns, endpointID)
	s.mu.Unlock()
	s.logger.Info("USP Endpoint disconnected", "endpoint_id", endpointID)
}

func (s *Service) HandleMessage(ctx context.Context, endpointID string, msg *usp.Msg) error {
	if msg.Header == nil {
		return fmt.Errorf("USP message header is nil")
	}

	s.logger.Info("USP message received", "endpoint_id", endpointID, "msg_type", msg.Header.MsgType)

	// TODO: Route USP Message to appropriate controller logic
	// For now, this just acts as a receiver mockup

	return nil
}
