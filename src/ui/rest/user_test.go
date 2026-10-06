package rest

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	domainUser "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/user"
	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/ui/rest/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SAYWHAT-PATCH: user-check-jid.
type checkUserServiceStub struct {
	domainUser.IUserUsecase
	response domainUser.CheckResponse
	err      error
}

func (stub *checkUserServiceStub) IsOnWhatsApp(_ context.Context, _ domainUser.CheckRequest) (domainUser.CheckResponse, error) {
	return stub.response, stub.err
}

func getUserCheck(t *testing.T, service domainUser.IUserUsecase) (int, map[string]any) {
	t.Helper()
	app := fiber.New()
	app.Use(middleware.Recovery())
	InitRestUser(app, service)

	response, err := app.Test(httptest.NewRequest("GET", "/user/check?phone=525512345678", nil))
	require.NoError(t, err)
	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body), string(raw))
	return response.StatusCode, body
}

func TestUserCheckRouteReturnsTheJID(t *testing.T) {
	status, body := getUserCheck(t, &checkUserServiceStub{
		response: domainUser.CheckResponse{IsOnWhatsApp: true, JID: "5215512345678@s.whatsapp.net"},
	})

	require.Equal(t, fiber.StatusOK, status)
	assert.Equal(t, map[string]any{"is_on_whatsapp": true, "jid": "5215512345678@s.whatsapp.net"}, body["results"])
}

// A failed lookup is an error envelope, never a 200 saying false.
func TestUserCheckRouteRendersALookupFailureAsAnError(t *testing.T) {
	status, body := getUserCheck(t, &checkUserServiceStub{
		err: pkgError.InternalServerError("could not check whether +525512345678 is on WhatsApp: timed out"),
	})

	assert.Equal(t, fiber.StatusInternalServerError, status)
	assert.Equal(t, "INTERNAL_SERVER_ERROR", body["code"])
	assert.NotContains(t, body, "results")
}
