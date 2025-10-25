package rms

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
)

type Subscription struct {
	SubId     string `json:"subId"`
	UeId      string `json:"ueId"`
	NotifyUri string `json:"notifyUri"`
}

type SubscriptionStore struct {
	sync.RWMutex
	ByID map[string]Subscription // Key: SubId
	ByUE map[string][]string     // Key: UeId, Value: List of SubIds
}

func NewSubscriptionStore() *SubscriptionStore {
	return &SubscriptionStore{
		ByID: make(map[string]Subscription),
		ByUE: make(map[string][]string),
	}
}

// CreateSubscription (POST)
func (s *SubscriptionStore) CreateSubscription(ueID, notifyURI string) (Subscription, error) {
	s.Lock()
	defer s.Unlock()

	subID := uuid.New().String()
	sub := Subscription{SubId: subID, UeId: ueID, NotifyUri: notifyURI}

	s.ByID[subID] = sub
	s.ByUE[ueID] = append(s.ByUE[ueID], subID)

	return sub, nil
}

// GetAllSubscriptions (GET)
func (s *SubscriptionStore) GetAllSubscriptions() []Subscription {
	s.RLock()
	defer s.RUnlock()
	list := make([]Subscription, 0, len(s.ByID))
	for _, sub := range s.ByID {
		list = append(list, sub)
	}
	return list
}

// UpsertSubscription (PUT)
func (s *SubscriptionStore) UpsertSubscription(subID string, sub Subscription) (isNew bool, err error) {
	s.Lock()
	defer s.Unlock()

	sub.SubId = subID

	oldSub, exists := s.ByID[subID]

	if !exists {
		s.ByUE[sub.UeId] = append(s.ByUE[sub.UeId], subID)
		isNew = true
	} else {
		if oldSub.UeId != sub.UeId {
			s.removeSubIdFromUeIndex(oldSub.UeId, subID)
			s.ByUE[sub.UeId] = append(s.ByUE[sub.UeId], subID)
		}
	}

	s.ByID[subID] = sub
	return isNew, nil
}

// DeleteSubscription (DELETE)
func (s *SubscriptionStore) DeleteSubscription(subID string) error {
	s.Lock()
	defer s.Unlock()

	oldSub, exists := s.ByID[subID]
	if !exists {
		return fmt.Errorf("subscription ID %s not found", subID)
	}

	delete(s.ByID, subID)
	s.removeSubIdFromUeIndex(oldSub.UeId, subID)
	return nil
}

func (s *SubscriptionStore) FindByUeId(ueID string) []Subscription {
	s.RLock()
	defer s.RUnlock()

	subIDs, ok := s.ByUE[ueID]
	if !ok || len(subIDs) == 0 {
		return nil
	}

	list := make([]Subscription, 0, len(subIDs))
	for _, id := range subIDs {
		if sub, found := s.ByID[id]; found {
			list = append(list, sub)
		}
	}
	return list
}

func (s *SubscriptionStore) removeSubIdFromUeIndex(ueID, subID string) {
	if subIDs, ok := s.ByUE[ueID]; ok {
		for i, id := range subIDs {
			if id == subID {
				s.ByUE[ueID][i] = s.ByUE[ueID][len(s.ByUE[ueID])-1]
				s.ByUE[ueID] = s.ByUE[ueID][:len(s.ByUE[ueID])-1]
				if len(s.ByUE[ueID]) == 0 {
					delete(s.ByUE, ueID)
				}
				return
			}
		}
	}
}
