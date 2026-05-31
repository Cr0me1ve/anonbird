package auth

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/client/internal/anonymous"
	"github.com/netbirdio/netbird/client/internal/profilemanager"
)

func TestConfigureAnonymousPKCEConfigRejectsClearnetProvider(t *testing.T) {
	auth := &Auth{
		config: &profilemanager.Config{
			AnonymousMode: true,
			AnonymousTransport: anonymous.TransportConfig{
				Type:      anonymous.TransportTorRelayOnly,
				TorSOCKS5: "127.0.0.1:9050",
			},
		},
	}

	config := &PKCEAuthProviderConfig{
		TokenEndpoint:         "https://idp.example.com/token",
		AuthorizationEndpoint: "https://idpexample.onion/authorize",
	}

	err := auth.configureAnonymousPKCEConfig(config)
	require.Error(t, err)
	require.Contains(t, err.Error(), "PKCE token")
	require.Nil(t, config.HTTPClient)
}

func TestConfigureAnonymousPKCEConfigInstallsAnonymousHTTPClient(t *testing.T) {
	auth := &Auth{
		config: &profilemanager.Config{
			AnonymousMode: true,
			AnonymousTransport: anonymous.TransportConfig{
				Type:      anonymous.TransportTorRelayOnly,
				TorSOCKS5: "127.0.0.1:9050",
			},
		},
	}

	config := &PKCEAuthProviderConfig{
		TokenEndpoint:         "https://idpexample.onion/token",
		AuthorizationEndpoint: "https://idpexample.onion/authorize",
	}

	require.NoError(t, auth.configureAnonymousPKCEConfig(config))
	require.NotNil(t, config.HTTPClient)
}

func TestConfigureAnonymousDeviceConfigInstallsValidator(t *testing.T) {
	auth := &Auth{
		config: &profilemanager.Config{
			AnonymousMode: true,
			AnonymousTransport: anonymous.TransportConfig{
				Type:                  anonymous.TransportI2PDatagram,
				I2PSAM:                "127.0.0.1:7656",
				I2PTunnelLength:       1,
				I2PTunnelQuantity:     3,
				I2PDestinationPublic:  "public-destination",
				I2PDestinationPrivate: "private-destination",
			},
		},
	}

	config := &DeviceAuthProviderConfig{
		TokenEndpoint:      "https://idpexample.b32.i2p/token",
		DeviceAuthEndpoint: "https://idpexample.b32.i2p/device",
	}

	require.NoError(t, auth.configureAnonymousDeviceConfig(config))
	require.NotNil(t, config.HTTPClient)
	require.NotNil(t, config.EndpointValidator)
	require.NoError(t, config.EndpointValidator("device verification", "https://idpexample.b32.i2p/activate"))
	require.Error(t, config.EndpointValidator("device verification", "https://idpexample.onion/activate"))
	require.Error(t, config.EndpointValidator("device verification", "https://idp.example.com/activate"))
}

func TestDeviceAuthorizationFlowRejectsClearnetVerificationURI(t *testing.T) {
	httpClient := mockHTTPClient{
		resBody: `{"verification_uri":"https://idp.example.com/activate","expires_in":30,"interval":1}`,
		code:    http.StatusOK,
	}
	config := DeviceAuthProviderConfig{
		Audience:           "audience",
		ClientID:           "client-id",
		TokenEndpoint:      "https://idpexample.onion/token",
		DeviceAuthEndpoint: "https://idpexample.onion/device",
		Scope:              "openid",
		EndpointValidator: func(serviceName, endpoint string) error {
			return anonymous.ValidateEndpointForTransport(serviceName, endpoint, anonymous.TransportConfig{
				Type:      anonymous.TransportTorRelayOnly,
				TorSOCKS5: "127.0.0.1:9050",
			})
		},
	}

	deviceFlow, err := NewDeviceAuthorizationFlow(config)
	require.NoError(t, err)
	deviceFlow.HTTPClient = &httpClient

	_, err = deviceFlow.RequestAuthInfo(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "device verification")
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestPKCEWaitTokenUsesConfiguredHTTPClient(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/", listener.Addr().(*net.TCPAddr).Port)
	require.NoError(t, listener.Close())

	const audience = "client-id"
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"aud": audience})
	tokenString, err := token.SignedString([]byte("secret"))
	require.NoError(t, err)

	var tokenEndpointCalled bool
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			tokenEndpointCalled = true
			require.Equal(t, "idpexample.onion", req.URL.Host)
			body := fmt.Sprintf(`{"access_token":%q,"token_type":"Bearer"}`, tokenString)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}

	pkce, err := NewPKCEAuthorizationFlow(PKCEAuthProviderConfig{
		ClientID:              audience,
		TokenEndpoint:         "https://idpexample.onion/token",
		AuthorizationEndpoint: "https://idpexample.onion/authorize",
		Scope:                 "openid",
		RedirectURLs:          []string{redirectURL},
		HTTPClient:            httpClient,
	})
	require.NoError(t, err)
	authInfo, err := pkce.RequestAuthInfo(context.Background())
	require.NoError(t, err)

	authURL, err := url.Parse(authInfo.VerificationURIComplete)
	require.NoError(t, err)
	state := authURL.Query().Get(queryState)
	require.NotEmpty(t, state)

	resultChan := make(chan error, 1)
	go func() {
		_, err := pkce.WaitToken(context.Background(), AuthFlowInfo{ExpiresIn: 5})
		resultChan <- err
	}()

	callbackURL := fmt.Sprintf("%s?%s=%s&%s=code", redirectURL, queryState, url.QueryEscape(state), queryCode)
	var callbackErr error
	for i := 0; i < 50; i++ {
		var resp *http.Response
		resp, callbackErr = http.Get(callbackURL) //nolint:gosec
		if callbackErr == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.NoError(t, callbackErr)

	select {
	case err := <-resultChan:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for PKCE token exchange")
	}
	require.True(t, tokenEndpointCalled)
}
