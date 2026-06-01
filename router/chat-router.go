package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func SetChatRouter(root *gin.Engine, apiRouter *gin.RouterGroup) {
	chatRealtimeRoute := root.Group("/api/chat")
	chatRealtimeRoute.Use(middleware.RouteTag("api"))
	chatRealtimeRoute.Use(middleware.GlobalAPIRateLimit())
	{
		chatRealtimeRoute.GET("/ws", controller.ChatWebSocket)
	}

	chatRoute := apiRouter.Group("/chat")
	chatRoute.Use(middleware.UserAuth())
	{
		chatRoute.GET("/users", controller.ListChatUsers)
		chatRoute.GET("/conversations", controller.ListChatConversations)
		chatRoute.POST("/conversations/direct", controller.CreateDirectChatConversation)
		chatRoute.POST("/conversations/group", controller.CreateGroupChatConversation)
		chatRoute.GET("/conversations/:id/messages", controller.ListChatMessages)
		chatRoute.POST("/conversations/:id/messages", controller.SendChatMessage)
		chatRoute.POST("/conversations/:id/messages/:message_id/revoke", controller.RevokeChatMessage)
		chatRoute.POST("/conversations/:id/messages/:message_id/reactions", controller.ToggleChatMessageReaction)
		chatRoute.POST("/conversations/:id/read", controller.MarkChatRead)
		chatRoute.POST("/conversations/:id/members", controller.AddChatMember)
		chatRoute.DELETE("/conversations/:id/members/:user_id", controller.RemoveChatMember)
	}
}
