package client

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConnDeadlinesReturnError(t *testing.T) {
	conn := &Conn{}

	require.ErrorIs(t, conn.SetDeadline(time.Now()), errRelayConnDeadlineUnsupported)
	require.ErrorIs(t, conn.SetReadDeadline(time.Now()), errRelayConnDeadlineUnsupported)
	require.ErrorIs(t, conn.SetWriteDeadline(time.Now()), errRelayConnDeadlineUnsupported)
}
