package rms

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	amf_context "github.com/free5gc/amf/internal/context"
	"github.com/free5gc/amf/internal/gmm"
	amf_logger "github.com/free5gc/amf/internal/logger"
	"github.com/free5gc/util/fsm"
)

type UeRMNotif struct {
	SubId     string `json:"subId"`
	UeId      string `json:"ueId"`
	PrevState string `json:"from"`
	CurrState string `json:"to"`
}

type CustomizedRMS struct {
	// implement your customized RMS fields here
	store *SubscriptionStore
}

func NewRMS(
// implement your customized RMS initialization here
) fsm.RMS {
	storeI := amf_context.GetSelf().SubscriptionStore
	store, ok := storeI.(*SubscriptionStore)
	if !ok || store == nil {
		amf_logger.RmsLog.Errorln("RMS Subscription Store is nil or wrong type in Context")
		return nil
	}
	return &CustomizedRMS{
		store: store,
	}
}

func (rms *CustomizedRMS) HandleEvent(state *fsm.State, event fsm.EventType, args fsm.ArgsType, trans fsm.Transition) {
	// implement your customized RMS logic here
	ueI, ok := args[gmm.ArgAmfUe]
	if !ok {
		amf_logger.RmsLog.Warnln("AMF UE not found in args")
		return
	}
	ue, ok := ueI.(*amf_context.AmfUe)
	if !ok || ue.Supi == "" {
		amf_logger.RmsLog.Warnln("Invalid AMF UE in args")
		return
	}

	ueID := ue.Supi

	subscriptions := rms.store.FindByUeId(ueID)
	if len(subscriptions) == 0 {
		amf_logger.RmsLog.Tracef("No subscriptions for UE %s", ueID)
		return
	}

	prevState := string(trans.From)
	currState := string(trans.To)

	for _, sub := range subscriptions {
		go sendNotification(sub, ueID, prevState, currState)
	}
}

func sendNotification(sub Subscription, ueID, prevState, currState string) {
	notification := UeRMNotif{
		SubId:     sub.SubId,
		UeId:      ueID,
		PrevState: prevState,
		CurrState: currState,
	}

	body, err := json.Marshal(notification)
	if err != nil {
		amf_logger.RmsLog.Errorf("Failed to marshal notification: %+v", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, sub.NotifyUri, bytes.NewReader(body))
	if err != nil {
		amf_logger.RmsLog.Errorf("Failed to create notification request: %+v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := http.DefaultClient
	client.Timeout = 5 * time.Second

	resp, err := client.Do(req)
	if err != nil {
		amf_logger.RmsLog.Errorf("Failed to send notification to %s: %+v", sub.NotifyUri, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		io.Copy(io.Discard, resp.Body)
		amf_logger.RmsLog.Errorf("Notification to %s returned status %d", sub.NotifyUri, resp.StatusCode)
	} else {
		amf_logger.RmsLog.Infof("Notification sent to %s successfully", sub.NotifyUri)
	}
}
