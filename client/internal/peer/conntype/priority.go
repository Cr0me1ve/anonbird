package conntype

import (
	"fmt"
)

const (
	None        ConnPriority = 0
	Relay       ConnPriority = 1
	I2PDatagram ConnPriority = 2
	ICETurn     ConnPriority = 3
	ICEP2P      ConnPriority = 4
)

type ConnPriority int

func (cp ConnPriority) String() string {
	switch cp {
	case None:
		return "None"
	case Relay:
		return "PriorityRelay"
	case I2PDatagram:
		return "PriorityI2PDatagram"
	case ICETurn:
		return "PriorityICETurn"
	case ICEP2P:
		return "PriorityICEP2P"
	default:
		return fmt.Sprintf("ConnPriority(%d)", cp)
	}
}
