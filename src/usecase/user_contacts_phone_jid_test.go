package usecase

// SAYWHAT-PATCH: contacts-phone-jid. An @lid contact carries the phone JID the
// chat list is keyed by, so saywhat opens the existing chat rather than a
// second one. Only @lid entries are looked up, and a missing or failed lookup
// leaves the field out instead of failing the list.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	domainUser "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow/types"
)

func fakePNForLID(lookups *[]types.JID, mappings map[types.JID]types.JID, err error) pnForLIDFunc {
	return func(_ context.Context, lid types.JID) (types.JID, error) {
		*lookups = append(*lookups, lid)
		return mappings[lid], err
	}
}

func TestContactPhoneJIDResolvesAMappedLID(t *testing.T) {
	lid := types.NewJID("123456789012345", types.HiddenUserServer)
	var lookups []types.JID
	getPN := fakePNForLID(&lookups, map[types.JID]types.JID{
		lid: types.NewJID("573001234567", types.DefaultUserServer),
	}, nil)

	got := contactPhoneJID(context.Background(), getPN, lid)

	assert.Equal(t, "573001234567@s.whatsapp.net", got)
	assert.Equal(t, []types.JID{lid}, lookups)

	body, err := json.Marshal(domainUser.MyListContactsResponseData{JID: lid, Name: "Ana", PhoneJID: got})
	require.NoError(t, err)
	assert.JSONEq(t, `{"jid":"123456789012345@lid","name":"Ana","phone_jid":"573001234567@s.whatsapp.net"}`, string(body))
}

func TestContactPhoneJIDOmitsAnUnmappedLID(t *testing.T) {
	cases := map[string]error{
		"no mapping":    nil,
		"lookup failed": errors.New("database is locked"),
	}
	for name, lookupErr := range cases {
		t.Run(name, func(t *testing.T) {
			lid := types.NewJID("999999999999999", types.HiddenUserServer)
			var lookups []types.JID

			got := contactPhoneJID(context.Background(), fakePNForLID(&lookups, nil, lookupErr), lid)

			assert.Empty(t, got)
			assert.Len(t, lookups, 1)
			body, err := json.Marshal(domainUser.MyListContactsResponseData{JID: lid, Name: "Unknown", PhoneJID: got})
			require.NoError(t, err)
			assert.NotContains(t, string(body), "phone_jid")
		})
	}
}

func TestContactPhoneJIDLeavesAPhoneEntryAlone(t *testing.T) {
	var lookups []types.JID
	getPN := fakePNForLID(&lookups, nil, nil)

	got := contactPhoneJID(context.Background(), getPN, types.NewJID("573001234567", types.DefaultUserServer))

	assert.Empty(t, got)
	assert.Empty(t, lookups, "a phone JID is never looked up")
}
