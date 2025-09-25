// DO NOT EDIT
package rsm_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/h2non/gock"

	amf_context "github.com/free5gc/amf/internal/context"
	"github.com/free5gc/amf/internal/gmm"
	amf_logger "github.com/free5gc/amf/internal/logger"
	rsm "github.com/free5gc/amf/internal/rms"
	"github.com/free5gc/amf/pkg/factory"
	amf_service "github.com/free5gc/amf/pkg/service"
	"github.com/free5gc/openapi/models"
	"github.com/free5gc/util/fsm"
)

// Models as defined in the assignment spec
type Subscription struct {
	SubId     string `json:"subId"`
	UeId      string `json:"ueId"`
	NotifyUri string `json:"notifyUri"`
}

type UeRMNotif struct {
	SubId     string `json:"subId"`
	UeId      string `json:"ueId"`
	PrevState string `json:"from"`
	CurrState string `json:"to"`
}

var (
	testAMF    *amf_service.AmfApp
	baseAPIURL string
)

// TestMain spins up a lightweight AMF instance exposing the SBI server (HTTP) including namf-rmm routes.
func TestMain(m *testing.M) {
	// Build a minimal config (do not call Validate - we intentionally include "namf-rmm").
	factory.AmfConfig = &factory.Config{
		Info: &factory.Info{Version: "1.0.9", Description: "test cfg"},
		Configuration: &factory.Configuration{
			AmfName:    "AMF-TEST",
			NgapIpList: []string{"127.0.0.1"},
			NgapPort:   38412,
			Sbi: &factory.Sbi{
				Scheme:       "http",
				RegisterIPv4: "127.0.0.15",
				BindingIPv4:  "127.0.0.15",
				Port:         18080,
				Tls:          &factory.Tls{Pem: "cert/amf.pem", Key: "cert/amf.key"},
			},
			// Include namf-rmm so the router mounts the routes under /namf-rmm/v1
			ServiceNameList: []string{
				"namf-comm", "namf-evts", "namf-mt", "namf-loc", "namf-oam", "namf-rmm",
			},
			ServedGumaiList: []models.Guami{{
				PlmnId: &models.PlmnIdNid{Mcc: "208", Mnc: "93"},
				AmfId:  "cafe00",
			}},
			SupportTAIList: []models.Tai{{
				PlmnId: &models.PlmnId{Mcc: "208", Mnc: "93"},
				Tac:    "000001",
			}},
			PlmnSupportList: []factory.PlmnSupportItem{{
				PlmnId:     &models.PlmnId{Mcc: "208", Mnc: "93"},
				SNssaiList: []models.Snssai{{Sst: 1, Sd: "fedcba"}},
			}},
			SupportDnnList: []string{"internet"},
			NrfUri:         "http://127.0.0.10:8000",
			Security: &factory.Security{
				IntegrityOrder: []string{"NIA2"},
				CipheringOrder: []string{"NEA0"},
			},
			NetworkName: factory.NetworkName{Full: "free5GC", Short: "free"},
			T3502Value:  720, T3512Value: 3600, Non3gppDeregTimerValue: 3240,
			T3513: factory.TimerValue{Enable: true},
			T3522: factory.TimerValue{Enable: true},
			T3550: factory.TimerValue{Enable: true},
			T3560: factory.TimerValue{Enable: true},
			T3565: factory.TimerValue{Enable: true},
			T3570: factory.TimerValue{Enable: true},
			T3555: factory.TimerValue{Enable: true},
		},
		Logger: &factory.Logger{Enable: false, Level: "info", ReportCaller: false},
	}

	// Spin up AMF
	ctx := context.Background()
	var err error
	testAMF, err = amf_service.NewApp(ctx, factory.AmfConfig, "")
	if err != nil {
		fmt.Printf("failed to create AMF app: %v\n", err)
		os.Exit(1)
	}

	baseAPIURL = fmt.Sprintf("http://%s:%d%s", factory.AmfConfig.Configuration.Sbi.RegisterIPv4, factory.AmfConfig.Configuration.Sbi.Port, factory.AmfRmmResUriPrefix)

	go testAMF.Start()

	// Wait for server to be ready by probing root
	if err := waitHTTPReady(baseAPIURL+"/", 5*time.Second); err != nil {
		fmt.Printf("AMF SBI not ready: %v\n", err)
		// continue anyway; tests will fail if endpoints are not mounted
	}

	code := m.Run()

	// Teardown
	if testAMF != nil {
		testAMF.Terminate()
		// give it a moment to shut down
		time.Sleep(200 * time.Millisecond)
	}
	os.Exit(code)
}

func waitHTTPReady(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp != nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", url)
}

// --- HTTP helpers ---
func httpDoJSON(t *testing.T, method, url string, in any) (int, []byte) {
	t.Helper()
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	newClient := &http.Client{}
	resp, err := newClient.Do(req)
	if err != nil {
		t.Fatalf("http do: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

// --- CRUD tests per spec ---

func TestRMM_CRUD_Subscriptions(t *testing.T) {
	// Create
	subID := "sub-001"
	ueID := "imsi-208930000000001"
	notify := "http://127.0.0.1:9099/rmm-notify"
	reqSub := Subscription{UeId: ueID, NotifyUri: notify}
	status, data := httpDoJSON(t, http.MethodPost, fmt.Sprintf("%s/subscriptions/%s", baseAPIURL, subID), reqSub)
	if status != http.StatusCreated {
		t.Fatalf("POST want 201, got %d body=%s", status, string(data))
	}
	var created Subscription
	_ = json.Unmarshal(data, &created)

	// Get collection
	status, data = httpDoJSON(t, http.MethodGet, fmt.Sprintf("%s/subscriptions", baseAPIURL), nil)
	if status != http.StatusOK {
		t.Fatalf("GET want 200, got %d body=%s", status, string(data))
	}
	var list struct {
		Subscriptions []Subscription `json:"subscriptions"`
	}
	_ = json.Unmarshal(data, &list)

	// Update via PUT
	updated := Subscription{UeId: ueID, NotifyUri: notify + "/v2"}
	status, data = httpDoJSON(t, http.MethodPut, fmt.Sprintf("%s/subscriptions/%s", baseAPIURL, subID), updated)
	if status != http.StatusOK && status != http.StatusCreated {
		t.Fatalf("PUT want 200 or 201, got %d body=%s", status, string(data))
	}

	// Delete
	status, data = httpDoJSON(t, http.MethodDelete, fmt.Sprintf("%s/subscriptions/%s", baseAPIURL, subID), nil)
	if status != http.StatusNoContent {
		t.Fatalf("DELETE want 204, got %d body=%s", status, string(data))
	}
}

// Notification test: when UE FSM state changes, AMF should notify the consumer.
func TestRMM_Notification_OnGmmTransition(t *testing.T) {
	gock.InterceptClient(http.DefaultClient)
	defer gock.RestoreClient(http.DefaultClient)
	defer gock.Off()

	// Prepare a subscription for the UE
	ueID := "imsi-208930000000777"
	notifyBase := "http://127.0.0.1:9099"
	notifyPath := "/callback/rmm"

	// Create subscription via SBI
	reqSub := Subscription{UeId: ueID, NotifyUri: notifyBase + notifyPath}
	status, data := httpDoJSON(t, http.MethodPost, fmt.Sprintf("%s/subscriptions/%s", baseAPIURL, ""), reqSub)
	if status != http.StatusCreated {
		t.Fatalf("POST subscription want 201, got %d body=%s", status, string(data))
	}
	var subs Subscription
	_ = json.Unmarshal(data, &subs)
	subID := subs.SubId
	// Expect a POST to the callback URI with a payload including subId and ueId
	gock.New(notifyBase).
		Post(notifyPath).
		MatchType("json").
		JSON(&UeRMNotif{SubId: subID, UeId: ueID, PrevState: "Deregistered", CurrState: "Authentication"}).
		Reply(204)

	// Attach our RMS (student implementation should use it to send notification)
	gmm.AttachRSM(rsm.NewRMS())

	// Create a UE context and trigger a GMM transition
	ue := amf_context.GetSelf().NewAmfUe(ueID)
	anType := models.AccessType__3_GPP_ACCESS
	ue.State[anType] = fsm.NewState("Deregistered")

	// Trigger from Deregistered -> Authentication (StartAuthEvent)
	if err := gmm.GmmFSM.SendEvent(ue.State[anType], gmm.StartAuthEvent, fsm.ArgsType{gmm.ArgAmfUe: ue, gmm.ArgAccessType: anType}, amf_logger.GmmLog); err != nil {
		t.Fatalf("SendEvent StartAuthEvent failed: %v", err)
	}

	// Give some time for async handlers to run
	waitUntil := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(waitUntil) && !gock.IsDone() {
		time.Sleep(20 * time.Millisecond)
	}

	if !gock.IsDone() {
		t.Fatalf("expected notification not received by callback server")
	}
}
