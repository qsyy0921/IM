package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/push-gateway/internal/app"
	"github.com/qsyy0921/IM/services/push-gateway/internal/domain"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
	nhooyr "nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type Server struct {
	connect    *app.ConnectSessionUseCase
	disconnect *app.DisconnectSessionUseCase
	frames     *app.HandleClientFrameUseCase
	config     Config
}

type Config struct {
	QueueSize         int
	HeartbeatInterval time.Duration
	WriteTimeout      time.Duration
}

func NewServer(
	connect *app.ConnectSessionUseCase,
	disconnect *app.DisconnectSessionUseCase,
	frames *app.HandleClientFrameUseCase,
	config Config,
) *Server {
	if config.QueueSize <= 0 {
		config.QueueSize = types.DefaultSessionQueueSize
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = types.DefaultHeartbeatInterval
	}
	if config.WriteTimeout <= 0 {
		config.WriteTimeout = 2 * time.Second
	}
	return &Server{connect: connect, disconnect: disconnect, frames: frames, config: config}
}

func (server *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	conn, err := nhooyr.Accept(writer, request, &nhooyr.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.Close(nhooyr.StatusNormalClosure, "")

	auth, err := authFromRequest(request)
	if err != nil {
		_ = wsjson.Write(request.Context(), conn, app.PublicErrorFrame("", err))
		return
	}
	var rawHello json.RawMessage
	if err := wsjson.Read(request.Context(), conn, &rawHello); err != nil {
		return
	}
	helloFrame, requestID, err := DecodeClientFrame(rawHello)
	if err != nil {
		_ = wsjson.Write(request.Context(), conn, app.PublicErrorFrame(requestID, err))
		return
	}
	if helloFrame.Op != types.OpClientHello {
		_ = wsjson.Write(request.Context(), conn, app.PublicErrorFrame(helloFrame.RequestID, types.NewInvalidFrame("client.hello is required")))
		return
	}
	if auth.DeviceID == "" {
		auth.DeviceID = helloFrame.DeviceID
	} else if helloFrame.DeviceID != "" && auth.DeviceID != helloFrame.DeviceID {
		_ = wsjson.Write(request.Context(), conn, app.PublicErrorFrame(helloFrame.RequestID, types.NewInvalidFrame("device_id mismatch")))
		return
	}
	if err := auth.Validate(); err != nil {
		_ = wsjson.Write(request.Context(), conn, app.PublicErrorFrame(helloFrame.RequestID, err))
		return
	}

	outbound := make(chan types.ServerFrame, server.config.QueueSize)
	evicted := make(chan types.SessionEviction, 1)
	result, err := server.connect.Execute(request.Context(), types.ConnectSessionCommand{
		AuthContext:       auth,
		QueueSize:         server.config.QueueSize,
		HeartbeatInterval: server.config.HeartbeatInterval,
	}, outbound, evicted)
	if err != nil {
		_ = wsjson.Write(request.Context(), conn, app.PublicErrorFrame("", err))
		return
	}
	disconnected := false
	disconnect := func() {
		if disconnected {
			return
		}
		disconnected = true
		server.disconnect.Execute(result.SessionID)
	}
	defer disconnect()

	auth.SessionID = result.SessionID
	if err := writeFrame(request.Context(), conn, domain.ServerHello(helloFrame.RequestID, result), server.config.WriteTimeout); err != nil {
		return
	}

	sessionCtx, cancelSession := context.WithCancel(request.Context())
	defer cancelSession()

	writeDone := make(chan error, 1)
	go func() {
		err := writeLoop(sessionCtx, conn, outbound, evicted, server.config.WriteTimeout)
		cancelSession()
		writeDone <- err
	}()

	readErr := server.readLoop(sessionCtx, conn, auth, outbound)
	cancelSession()
	disconnect()
	writeErr := <-writeDone
	if readErr != nil && !isNormalClose(readErr) {
		return
	}
	if writeErr != nil && !isNormalClose(writeErr) {
		return
	}
}

func (server *Server) readLoop(
	ctx context.Context,
	conn *nhooyr.Conn,
	auth types.AuthContext,
	outbound chan<- types.ServerFrame,
) error {
	for {
		var raw json.RawMessage
		if err := wsjson.Read(ctx, conn, &raw); err != nil {
			return err
		}
		frame, requestID, err := DecodeClientFrame(raw)
		if err != nil {
			if err := enqueueOutbound(ctx, outbound, app.PublicErrorFrame(requestID, err)); err != nil {
				return err
			}
			continue
		}
		response, err := server.frames.Execute(ctx, auth, frame)
		if err != nil {
			if err := enqueueOutbound(ctx, outbound, app.PublicErrorFrame(frame.RequestID, err)); err != nil {
				return err
			}
			continue
		}
		if response.Op != "" {
			if err := enqueueOutbound(ctx, outbound, response); err != nil {
				return err
			}
		}
	}
}

func enqueueOutbound(ctx context.Context, outbound chan<- types.ServerFrame, frame types.ServerFrame) error {
	select {
	case outbound <- frame:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func writeLoop(
	ctx context.Context,
	conn *nhooyr.Conn,
	outbound <-chan types.ServerFrame,
	evicted <-chan types.SessionEviction,
	timeout time.Duration,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case eviction, ok := <-evicted:
			if !ok {
				evicted = nil
				continue
			}
			_ = writeFrame(ctx, conn, domain.ResumeHint(eviction.Reason, eviction.Conversations), timeout)
			_ = conn.Close(nhooyr.StatusPolicyViolation, "slow session")
			return types.ErrSessionEvicted
		case frame, ok := <-outbound:
			if !ok {
				return nil
			}
			if err := writeFrame(ctx, conn, frame, timeout); err != nil {
				return err
			}
		}
	}
}

func writeFrame(ctx context.Context, conn *nhooyr.Conn, frame types.ServerFrame, timeout time.Duration) error {
	writeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return wsjson.Write(writeCtx, conn, frame)
}

func DecodeClientFrame(raw json.RawMessage) (types.ClientFrame, string, error) {
	var probe struct {
		Op        string `json:"op"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return types.ClientFrame{}, "", types.NewInvalidFrame("invalid json")
	}
	var frame types.ClientFrame
	if err := json.Unmarshal(raw, &frame); err != nil {
		return types.ClientFrame{}, probe.RequestID, types.NewInvalidFrame("invalid frame")
	}
	switch frame.Op {
	case types.OpClientHello:
		if frame.DeviceID == "" {
			return types.ClientFrame{}, frame.RequestID, types.NewInvalidFrame("device_id is required")
		}
	case types.OpClientPing:
	case types.OpDeliveryAck:
		if frame.ConversationID == "" || frame.ReceivedSeq <= 0 {
			return types.ClientFrame{}, frame.RequestID, types.NewInvalidFrame("delivery ack is incomplete")
		}
	default:
		return types.ClientFrame{}, frame.RequestID, types.NewInvalidFrame("unsupported op")
	}
	return frame, frame.RequestID, nil
}

func authFromRequest(request *http.Request) (types.AuthContext, error) {
	query := request.URL.Query()
	auth := types.AuthContext{
		TenantID: query.Get("tenant_id"),
		UserID:   query.Get("user_id"),
		DeviceID: query.Get("device_id"),
		TraceID:  query.Get("trace_id"),
	}
	if token := strings.TrimSpace(query.Get("token")); token != "" && (auth.TenantID == "" || auth.UserID == "") {
		parts := strings.Split(token, ":")
		if len(parts) >= 2 {
			auth.TenantID = parts[0]
			auth.UserID = parts[1]
		}
	}
	if auth.TenantID == "" {
		return types.AuthContext{}, types.NewInvalidFrame("tenant_id is required")
	}
	if auth.UserID == "" {
		return types.AuthContext{}, types.NewInvalidFrame("user_id is required")
	}
	return auth, nil
}

func isNormalClose(err error) bool {
	if err == nil {
		return true
	}
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, types.ErrSessionEvicted) ||
		nhooyr.CloseStatus(err) == nhooyr.StatusNormalClosure ||
		nhooyr.CloseStatus(err) == nhooyr.StatusGoingAway
}
