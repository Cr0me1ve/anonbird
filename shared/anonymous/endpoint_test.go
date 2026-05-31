package anonymous

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEndpointTransport(t *testing.T) {
	require.Equal(t, EndpointTransportTor, EndpointTransport("https://idpexample.onion/keys"))
	require.Equal(t, EndpointTransportTor, EndpointTransport("idpexample.onion:443"))
	require.Equal(t, EndpointTransportI2P, EndpointTransport("https://idpexample.b32.i2p/.well-known/openid-configuration"))
	require.Empty(t, EndpointTransport("https://idp.example.com/keys"))
	require.Empty(t, EndpointTransport("127.0.0.1:8080"))
}

func TestHTTPClientForEndpointBuildsTransport(t *testing.T) {
	client, err := HTTPClientForEndpoint("https://idpexample.onion/keys", time.Second)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, time.Second, client.Timeout)
	require.IsType(t, &http.Transport{}, client.Transport)

	client, err = HTTPClientForEndpoint("https://idpexample.b32.i2p/keys", time.Second)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, time.Second, client.Timeout)
	require.IsType(t, &http.Transport{}, client.Transport)
}
