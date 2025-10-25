package sbi

import (
	"fmt"
	"net/http"

	amf_context "github.com/free5gc/amf/internal/context"
	amf_logger "github.com/free5gc/amf/internal/logger"
	"github.com/free5gc/amf/internal/rms"
	"github.com/gin-gonic/gin"
)

type Subscription struct {
	SubId     string `json:"subId"`
	UeId      string `json:"ueId"`
	NotifyUri string `json:"notifyUri"`
}

func handleRMSNotFound(c *gin.Context, err error) {
	amf_logger.SBILog.Warnf("RM Monitoring API error: %v", err)
	c.AbortWithStatus(http.StatusNotFound) // 404
}

func (s *Server) getRMSRoutes() []Route {
	return []Route{
		{
			Name:    "root",
			Method:  http.MethodGet,
			Pattern: "/",
			APIFunc: func(c *gin.Context) {
				c.String(http.StatusOK, "Hello World!")
			},
		},
		// add more Route based on provided spec
		{
			Name:    "subscriptions_collection_get",
			Method:  http.MethodGet,
			Pattern: "/subscriptions",
			APIFunc: s.HandleGetSubscriptions,
		},
		{
			Name:    "subscriptions_collection_post",
			Method:  http.MethodPost,
			Pattern: "/subscriptions/",
			APIFunc: s.HandlePostSubscriptions,
		},
		{
			Name:    "subscription_document_put",
			Method:  http.MethodPut,
			Pattern: "/subscriptions/:subscriptionID",
			APIFunc: s.HandlePutSubscription,
		},
		{
			Name:    "subscription_document_delete",
			Method:  http.MethodDelete,
			Pattern: "/subscriptions/:subscriptionID",
			APIFunc: s.HandleDeleteSubscription,
		},
	}
}

// GET /subscriptions
func (s *Server) HandleGetSubscriptions(c *gin.Context) {
	subsList := amf_context.GetSelf().SubscriptionStore.GetAllSubscriptions()

	resp := struct {
		Subscriptions []rms.Subscription `json:"subscriptions"`
	}{
		Subscriptions: subsList,
	}
	c.JSON(http.StatusOK, resp)
}

// POST /subscriptions
func (s *Server) HandlePostSubscriptions(c *gin.Context) {
	var req Subscription
	if err := c.ShouldBindJSON(&req); err != nil {
		handleRMSNotFound(c, fmt.Errorf("invalid request body: %w", err))
		return
	}

	if req.UeId == "" || req.NotifyUri == "" {
		handleRMSNotFound(c, fmt.Errorf("missing UeId or NotifyUri in POST request"))
		return
	}

	sub, err := amf_context.GetSelf().SubscriptionStore.CreateSubscription(req.UeId, req.NotifyUri)
	if err != nil {
		handleRMSNotFound(c, err)
		return
	}

	c.JSON(http.StatusCreated, sub)
}

// PUT /subscriptions/{subscriptionID}
func (s *Server) HandlePutSubscription(c *gin.Context) {
	subID := c.Param("subscriptionID")
	var req Subscription
	if err := c.ShouldBindJSON(&req); err != nil {
		handleRMSNotFound(c, fmt.Errorf("invalid request body: %w", err))
		return
	}

	if req.UeId == "" || req.NotifyUri == "" {
		handleRMSNotFound(c, fmt.Errorf("missing UeId or NotifyUri in PUT request"))
		return
	}

	sub := rms.Subscription{UeId: req.UeId, NotifyUri: req.NotifyUri}

	isNew, err := amf_context.GetSelf().SubscriptionStore.UpsertSubscription(subID, sub)
	if err != nil {
		handleRMSNotFound(c, err)
		return
	}

	if isNew {
		c.JSON(http.StatusCreated, rms.Subscription{SubId: subID, UeId: req.UeId, NotifyUri: req.NotifyUri}) // 201 Created
	} else {
		c.JSON(http.StatusOK, rms.Subscription{SubId: subID, UeId: req.UeId, NotifyUri: req.NotifyUri}) // 200 OK
	}
}

// DELETE /subscriptions/{subscriptionID}
func (s *Server) HandleDeleteSubscription(c *gin.Context) {
	subID := c.Param("subscriptionID")

	err := amf_context.GetSelf().SubscriptionStore.DeleteSubscription(subID)
	if err != nil {
		handleRMSNotFound(c, err)
		return
	}

	c.Status(http.StatusNoContent) // 204 No Content
}
