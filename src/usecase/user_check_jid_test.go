package usecase

// SAYWHAT-PATCH: user-check-jid. /user/check must hand back the address
// WhatsApp answered with, not the typed digits, and must fail — not answer
// "not on WhatsApp" — when the lookup itself fails. saywhat builds chats from
// that JID and tells the user "could not check" on an error.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func fakeIsOnWhatsApp(queries *[]string, data []types.IsOnWhatsAppResponse, err error) func(context.Context, *whatsmeow.Client, []string) ([]types.IsOnWhatsAppResponse, error) {
	return func(_ context.Context, _ *whatsmeow.Client, phones []string) ([]types.IsOnWhatsAppResponse, error) {
		*queries = append(*queries, phones...)
		return data, err
	}
}

// Mexico: the typed 52… number is registered as 521…. The response carries
// the canonical JID, without the device part WhatsApp may attach.
func TestUserCheckReturnsTheCanonicalJIDForARegisteredNumber(t *testing.T) {
	var queries []string
	canonical := types.JID{User: "5215512345678", Server: types.DefaultUserServer, Device: 3}
	service := serviceUser{isOnWhatsAppFn: fakeIsOnWhatsApp(&queries, []types.IsOnWhatsAppResponse{
		{Query: "+525512345678", JID: canonical, IsIn: true},
	}, nil)}

	response, err := service.checkOnWhatsApp(context.Background(), nil, "525512345678")

	require.NoError(t, err)
	assert.Equal(t, []string{"+525512345678"}, queries)
	assert.True(t, response.IsOnWhatsApp)
	assert.Equal(t, "5215512345678@s.whatsapp.net", response.JID)

	body, err := json.Marshal(response)
	require.NoError(t, err)
	assert.JSONEq(t, `{"is_on_whatsapp":true,"jid":"5215512345678@s.whatsapp.net"}`, string(body))
}

// What the pinned whatsmeow actually answers: it queries in LID addressing
// mode, so JID is the @lid and the phone form arrives as PhoneNumber. The
// first deploy returned the @lid; the contract is the phone JID.
func TestUserCheckReturnsThePhoneJIDWhenWhatsAppAnswersWithALID(t *testing.T) {
	var queries []string
	service := serviceUser{isOnWhatsAppFn: fakeIsOnWhatsApp(&queries, []types.IsOnWhatsAppResponse{{
		Query:       "+525512345678",
		JID:         types.NewJID("148812093993177", types.HiddenUserServer),
		PhoneNumber: types.NewJID("5215512345678", types.DefaultUserServer),
		IsIn:        true,
	}}, nil)}

	response, err := service.checkOnWhatsApp(context.Background(), nil, "525512345678")

	require.NoError(t, err)
	assert.True(t, response.IsOnWhatsApp)
	assert.Equal(t, "5215512345678@s.whatsapp.net", response.JID)
}

// Registered, but WhatsApp gave no phone form: the @lid is never handed out
// as the jid, since opening it would start a second chat.
func TestUserCheckNeverReturnsALIDAsTheJID(t *testing.T) {
	var queries []string
	service := serviceUser{isOnWhatsAppFn: fakeIsOnWhatsApp(&queries, []types.IsOnWhatsAppResponse{{
		Query: "+525512345678",
		JID:   types.NewJID("148812093993177", types.HiddenUserServer),
		IsIn:  true,
	}}, nil)}

	response, err := service.checkOnWhatsApp(context.Background(), nil, "525512345678")

	require.NoError(t, err)
	assert.True(t, response.IsOnWhatsApp)
	assert.Empty(t, response.JID)
}

func TestUserCheckOmitsTheJIDForAnUnregisteredNumber(t *testing.T) {
	var queries []string
	service := serviceUser{isOnWhatsAppFn: fakeIsOnWhatsApp(&queries, []types.IsOnWhatsAppResponse{
		{Query: "+15550000000", JID: types.NewJID("15550000000", types.DefaultUserServer), IsIn: false},
	}, nil)}

	response, err := service.checkOnWhatsApp(context.Background(), nil, "15550000000")

	require.NoError(t, err)
	assert.False(t, response.IsOnWhatsApp)
	assert.Empty(t, response.JID)

	body, err := json.Marshal(response)
	require.NoError(t, err)
	assert.JSONEq(t, `{"is_on_whatsapp":false}`, string(body))
}

// Upstream folded a failed lookup into false. Both a whatsmeow error and an
// empty answer must now surface as an error the REST layer renders non-200.
func TestUserCheckFailsWhenTheLookupFails(t *testing.T) {
	cases := map[string]struct {
		data []types.IsOnWhatsAppResponse
		err  error
	}{
		"whatsmeow error": {err: errors.New("info query timed out")},
		"empty response":  {data: []types.IsOnWhatsAppResponse{}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var queries []string
			service := serviceUser{isOnWhatsAppFn: fakeIsOnWhatsApp(&queries, tc.data, tc.err)}

			response, err := service.checkOnWhatsApp(context.Background(), nil, "525512345678")

			require.Error(t, err)
			var generic pkgError.GenericError
			require.ErrorAs(t, err, &generic, "must be a pkgError so PanicIfNeeded renders the envelope")
			assert.GreaterOrEqual(t, generic.StatusCode(), 400)
			assert.False(t, response.IsOnWhatsApp)
			assert.Empty(t, response.JID)
		})
	}
}

// A blank phone used to fall through to the non-user branch and answer true.
func TestUserCheckRejectsABlankPhone(t *testing.T) {
	var queries []string
	service := serviceUser{isOnWhatsAppFn: fakeIsOnWhatsApp(&queries, nil, nil)}

	_, err := service.checkOnWhatsApp(context.Background(), nil, "  ")

	var validation pkgError.ValidationError
	require.ErrorAs(t, err, &validation)
	assert.Empty(t, queries, "nothing is sent to WhatsApp")
}
