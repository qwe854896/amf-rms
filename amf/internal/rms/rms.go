package rsm

import "github.com/free5gc/util/fsm"

type CustomizedRMS struct {
	// implement your customized RMS fields here
}

func NewRMS(
// implement your customized RMS initialization here
) fsm.RMS {
	return &CustomizedRMS{}
}

func (rms *CustomizedRMS) HandleEvent(state *fsm.State, event fsm.EventType, args fsm.ArgsType, trans fsm.Transition) {
	// implement your customized RMS logic here
}
