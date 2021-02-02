package sync

import (
	"context"
	"errors"
	"github.com/testground/testground/pkg/logging"
	"go.uber.org/zap"
	"net"
	"net/http"
	"nhooyr.io/websocket"
)

type Server struct {
	service Service
	server  *http.Server
	l       net.Listener
	log     *zap.SugaredLogger
}

func NewServer(service Service) (srv *Server, err error) {
	srv = &Server{
		service: service,
		server: &http.Server{
			Handler: http.HandlerFunc(srv.handler),
		},
		log: logging.S(),
	}

	srv.l, err = net.Listen("tcp", ":8080")
	if err != nil {
		return nil, err
	}

	return srv, err
}

func (s *Server) Serve() error {
	return s.server.Serve(s.l)
}

func (s *Server) Addr() string {
	return s.l.Addr().String()
}

func (s *Server) Port() int {
	return s.l.Addr().(*net.TCPAddr).Port
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) handler(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Accept requests from all domains.
	})
	if err != nil {
		s.log.Warnf("could not upgrade connection: %v", err)
		return
	}

	conn := &connection{
		Conn:      c,
		service:   s.service,
		ctx:       r.Context(),
		responses: make(chan *Response),
	}

	go func() {
		_ = conn.consumeResponses()
	}()
	err = conn.consumeRequests()

	if err == nil {
		_ = c.Close(websocket.StatusNormalClosure, "")
		return
	}

	if errors.Is(err, context.Canceled) ||
		websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
		websocket.CloseStatus(err) == websocket.StatusGoingAway {
		// Client closed the connection by itself.
		s.log.Info("client closed connection")
		_ = c.Close(websocket.StatusNormalClosure, "")
		return
	}

	s.log.Warnf(" websocket closed unexpectedly: %v", err)
	_ = c.Close(websocket.StatusInternalError, "")
}