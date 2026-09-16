package chat

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/model"
	authservice "github.com/QuantumNous/new-api/service"
	"github.com/centrifugal/centrifuge"
)

type RealtimeServer struct {
	node    *centrifuge.Node
	handler http.Handler
}

var (
	defaultRealtimeOnce   sync.Once
	defaultRealtimeServer *RealtimeServer
	defaultRealtimeErr    error
	defaultServiceOnce    sync.Once
	defaultService        *Service
	defaultServiceErr     error
)

func NewRealtimeServer() (*RealtimeServer, error) {
	node, err := centrifuge.New(centrifuge.Config{})
	if err != nil {
		return nil, err
	}
	node.OnConnecting(func(_ context.Context, event centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		identity, internal, err := authservice.ParseDashboardAccessToken(event.Token)
		if errors.Is(err, authservice.ErrAuthTokenExpired) {
			return centrifuge.ConnectReply{}, centrifuge.ErrorTokenExpired
		}
		if !internal || err != nil {
			return centrifuge.ConnectReply{}, centrifuge.ErrorUnauthorized
		}
		if _, _, err := authservice.ValidateLoginSession(identity); err != nil {
			return centrifuge.ConnectReply{}, centrifuge.ErrorUnauthorized
		}
		return centrifuge.ConnectReply{
			Credentials: &centrifuge.Credentials{UserID: strconv.Itoa(identity.UserID)},
		}, nil
	})
	node.OnConnect(func(client *centrifuge.Client) {
		client.OnSubscribe(func(event centrifuge.SubscribeEvent, callback centrifuge.SubscribeCallback) {
			if authorizeSubscription(client.UserID(), event.Channel) {
				callback(centrifuge.SubscribeReply{}, nil)
				return
			}
			callback(centrifuge.SubscribeReply{}, centrifuge.ErrorPermissionDenied)
		})
	})
	if err := node.Run(); err != nil {
		return nil, err
	}
	return &RealtimeServer{
		node:    node,
		handler: centrifuge.NewWebsocketHandler(node, centrifuge.WebsocketConfig{}),
	}, nil
}

func DefaultRealtimeServer() (*RealtimeServer, error) {
	defaultRealtimeOnce.Do(func() {
		defaultRealtimeServer, defaultRealtimeErr = NewRealtimeServer()
	})
	return defaultRealtimeServer, defaultRealtimeErr
}

func DefaultService() (*Service, error) {
	defaultServiceOnce.Do(func() {
		server, err := DefaultRealtimeServer()
		if err != nil {
			defaultServiceErr = err
			return
		}
		defaultService = NewService(NewCentrifugePublisher(server.Node()))
	})
	return defaultService, defaultServiceErr
}

func (server *RealtimeServer) Node() *centrifuge.Node {
	if server == nil {
		return nil
	}
	return server.node
}

func (server *RealtimeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if server == nil || server.handler == nil {
		http.Error(w, "chat realtime server is not initialized", http.StatusServiceUnavailable)
		return
	}
	server.handler.ServeHTTP(w, r)
}

func authorizeSubscription(userIDText string, channel string) bool {
	userID, err := strconv.Atoi(userIDText)
	if err != nil || userID <= 0 {
		return false
	}
	if channel == UserChannel(userID) {
		return true
	}
	if conversationID, ok := parseChannelID(channel, "conversation:"); ok {
		member, err := model.IsConversationMember(conversationID, userID)
		return err == nil && member
	}
	if conversationID, ok := parseChannelID(channel, "presence:"); ok {
		member, err := model.IsConversationMember(conversationID, userID)
		return err == nil && member
	}
	return false
}

func parseChannelID(channel string, prefix string) (int, bool) {
	if !strings.HasPrefix(channel, prefix) {
		return 0, false
	}
	id, err := strconv.Atoi(strings.TrimPrefix(channel, prefix))
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
