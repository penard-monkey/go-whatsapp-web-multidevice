package usecase

// SAYWHAT-PATCH: read-self-receipt. These pin the one property the patch
// exists for: MarkAsReadSelf hands whatsmeow exactly one receipt type, and it
// is read-self — never the plain read that becomes a blue tick when the
// account's read receipts are on, and never played, which whatsmeow does not
// downgrade at all.

import (
	"context"
	"testing"
	"time"

	domainMessage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func captureReceiptTypes(t *testing.T, calls *int, got *[]types.ReceiptType, sender *types.JID) markReadFunc {
	t.Helper()
	return func(_ context.Context, _ *whatsmeow.Client, _ []types.MessageID, _ time.Time, _ types.JID, s types.JID, receiptTypes ...types.ReceiptType) error {
		*calls++
		*got = receiptTypes
		*sender = s
		return nil
	}
}

func TestMarkAsReadSelfSendsOnlyReadSelfInADirectChat(t *testing.T) {
	service, _, ctx := newMessageActionTestService(t, nil)
	var calls int
	var got []types.ReceiptType
	var sender types.JID
	service.markReadFn = captureReceiptTypes(t, &calls, &got, &sender)

	response, err := service.MarkAsReadSelf(ctx, domainMessage.MarkAsReadRequest{
		MessageID: "message-1",
		Phone:     "628123456789@s.whatsapp.net",
	})

	require.NoError(t, err)
	require.Equal(t, 1, calls, "exactly one receipt")
	require.Equal(t, []types.ReceiptType{types.ReceiptTypeReadSelf}, got)
	assert.NotContains(t, got, types.ReceiptTypeRead)
	assert.NotContains(t, got, types.ReceiptTypePlayed)
	assert.NotContains(t, got, types.ReceiptTypePlayedSelf)
	assert.Equal(t, "message-1", response.MessageID)
}

// A group receipt needs the original sender as its participant. The self-only
// route must resolve it exactly as /read does, or groups would error (or name
// the wrong person) only on this path.
func TestMarkAsReadSelfResolvesTheGroupSenderLikeMarkAsRead(t *testing.T) {
	service, repo, ctx := newMessageActionTestService(t, nil)
	groupJID := types.NewJID("120363000000000100", types.GroupServer)
	senderJID := types.NewJID("628987654321", types.DefaultUserServer)
	storeGroupMessageForReadTest(t, repo, "device-a@s.whatsapp.net", groupJID, senderJID, "group-self-1")
	service.validateJIDFn = func(_ *whatsmeow.Client, _ string) (types.JID, error) {
		return groupJID, nil
	}

	var calls int
	var got []types.ReceiptType
	var sender types.JID
	service.markReadFn = captureReceiptTypes(t, &calls, &got, &sender)
	_, err := service.MarkAsReadSelf(ctx, domainMessage.MarkAsReadRequest{MessageID: "group-self-1", Phone: groupJID.String()})
	require.NoError(t, err)
	require.Equal(t, []types.ReceiptType{types.ReceiptTypeReadSelf}, got)
	selfSender := sender

	service.markReadFn = captureReceiptTypes(t, &calls, &got, &sender)
	_, err = service.MarkAsRead(ctx, domainMessage.MarkAsReadRequest{MessageID: "group-self-1", Phone: groupJID.String()})
	require.NoError(t, err)
	require.Empty(t, got, "plain /read is upstream's, unchanged")

	assert.Equal(t, senderJID, selfSender)
	assert.Equal(t, sender, selfSender)
}

// The guards in front of the receipt still apply: a message this device never
// stored is refused before anything is sent.
func TestMarkAsReadSelfRefusesAnUnknownGroupMessage(t *testing.T) {
	service, _, ctx := newMessageActionTestService(t, nil)
	groupJID := types.NewJID("120363000000000101", types.GroupServer)
	service.validateJIDFn = func(_ *whatsmeow.Client, _ string) (types.JID, error) {
		return groupJID, nil
	}
	service.markReadFn = failOnReadReceipt(t)

	_, err := service.MarkAsReadSelf(ctx, domainMessage.MarkAsReadRequest{MessageID: "never-stored", Phone: groupJID.String()})
	assert.ErrorContains(t, err, "not found for current device and chat")
}
